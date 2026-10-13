package api

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/core/notepayload"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

const maxBatchGetLinkMetadata = 10

func (s *APIService) CreateNote(ctx context.Context, request *apipb.CreateNoteRequest) (*apipb.Note, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}
	if request.Note == nil {
		return nil, status.Errorf(codes.InvalidArgument, "note is required")
	}

	noteUID, err := ValidateAndGenerateUID(request.NoteId)
	if err != nil {
		return nil, err
	}

	prepared, err := s.prepareNoteCreate(ctx, user, request.Note, noteUID)
	if err != nil {
		return nil, err
	}
	if err := s.checkNoteQuota(ctx, user); err != nil {
		return nil, err
	}

	if err := s.createNoteWithMutation(ctx, user, prepared.note, nil, prepared.attachments, prepared.requiredAttachmentIDs, prepared.referenceRelations); err != nil {
		return nil, mapNoteCreateError(err, noteUID, "failed to create note")
	}
	note := prepared.note

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get note attachments")
	}

	relations, err := s.loadNoteRelations(ctx, note)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load note relations")
	}
	noteMessage, err := s.convertNoteFromStore(ctx, note, nil, attachments, relations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert note")
	}
	// Try to dispatch webhook when note is created.
	if err := s.DispatchNoteCreatedWebhook(ctx, noteMessage); err != nil {
		slog.Warn("Failed to dispatch note created webhook", slog.Any("err", err))
	}

	s.SSEHub.publishNoteChanged()

	s.dispatchNoteMentionNotificationsBestEffort(ctx, note, nil, "")

	return noteMessage, nil
}

func (s *APIService) ListNotes(ctx context.Context, request *apipb.ListNotesRequest) (*apipb.ListNotesResponse, error) {
	noteFind := &store.FindNote{
		// Exclude comments by default.
		ExcludeComments: true,
	}
	accessScope, currentUser, err := s.resolveNoteAccessScope(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	// An anonymous caller may only list at all when the instance permits it.
	if currentUser == nil && !accessScope.AllowPublic {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	noteFind.Access = accessScope

	if request.State == apipb.State_ARCHIVED {
		state := store.Archived
		noteFind.RowStatus = &state
		// Archived notes are only visible to their creator.
		if currentUser == nil {
			return &apipb.ListNotesResponse{}, nil
		}
		noteFind.CreatorID = &currentUser.ID
	} else {
		state := store.Normal
		noteFind.RowStatus = &state
	}

	// Parse order_by field (replaces the old sort and direction fields)
	if request.OrderBy != "" {
		if err := s.parseNoteOrderBy(request.OrderBy, noteFind); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid order_by: %v", err)
		}
	} else {
		// Default ordering by create_time desc.
		noteFind.OrderByTimeAsc = false
	}

	if request.Filter != "" {
		if err := s.validateNoteFilterForUser(ctx, request.Filter, currentUser); err != nil {
			return nil, err
		}
		noteFind.Filters = append(noteFind.Filters, request.Filter)
	}

	var limit, offset int
	if request.PageToken != "" {
		var pageToken apipb.PageToken
		if err := unmarshalPageToken(request.PageToken, &pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = normalizePageSize(pageToken.Limit)
		offset = max(int(pageToken.Offset), 0)
	} else {
		limit = normalizePageSize(request.PageSize)
	}
	limit = min(limit, MaxPageSize)
	limitPlusOne := limit + 1
	noteFind.Limit = &limitPlusOne
	noteFind.Offset = &offset
	notes, err := s.Store.ListNotes(ctx, noteFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notes: %v", err)
	}

	noteMessages := []*apipb.Note{}
	nextPageToken := ""
	if len(notes) == limitPlusOne {
		notes = notes[:limit]
		nextPageToken, err = getPageToken(limit, offset+limit)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token, error: %v", err)
		}
	}

	if len(notes) == 0 {
		response := &apipb.ListNotesResponse{
			Notes:         noteMessages,
			NextPageToken: nextPageToken,
		}
		return response, nil
	}

	reactionMap := make(map[int32][]*store.Reaction)

	attachmentMap := make(map[int32][]*store.Attachment)
	noteIDs := make([]int32, 0, len(notes))

	for _, m := range notes {
		noteIDs = append(noteIDs, m.ID)
	}

	// REACTIONS
	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{NoteIDList: noteIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}
	for _, reaction := range reactions {
		reactionMap[reaction.NoteID] = append(reactionMap[reaction.NoteID], reaction)
	}

	// ATTACHMENTS
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteIDList: noteIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}
	for _, attachment := range attachments {
		attachmentMap[*attachment.NoteID] = append(attachmentMap[*attachment.NoteID], attachment)
	}

	// RELATIONS (batch load to avoid N+1)
	relationMap, err := s.batchConvertNoteRelations(ctx, notes, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to batch load note relations")
	}
	creatorIDs := make([]int32, 0, len(notes)+len(reactions))
	for _, note := range notes {
		creatorIDs = append(creatorIDs, note.CreatorID)
	}
	for _, reaction := range reactions {
		creatorIDs = append(creatorIDs, reaction.CreatorID)
	}
	creatorMap, err := s.listUsersByID(ctx, creatorIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list note creators: %v", err)
	}
	for _, note := range notes {
		reactions := reactionMap[note.ID]
		attachments := attachmentMap[note.ID]
		relations := relationMap[note.ID]

		noteMessage, err := s.convertNoteFromStoreWithCreators(ctx, note, reactions, attachments, relations, creatorMap)
		if err != nil {
			if stderrors.Is(err, errNoteCreatorNotFound) {
				slog.Warn("Skipping note with missing creator",
					slog.Int64("note_id", int64(note.ID)),
					slog.String("note_uid", note.UID),
					slog.Int64("creator_id", int64(note.CreatorID)),
				)
				continue
			}
			return nil, errors.Wrap(err, "failed to convert note")
		}

		noteMessages = append(noteMessages, noteMessage)
	}

	response := &apipb.ListNotesResponse{
		Notes:         noteMessages,
		NextPageToken: nextPageToken,
	}
	return response, nil
}

func (s *APIService) GetNote(ctx context.Context, request *apipb.GetNoteRequest) (*apipb.Note, error) {
	noteUID, err := ExtractNoteUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{
		UID: &noteUID,
	})
	if err != nil {
		return nil, err
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}

	relations, err := s.loadNoteRelations(ctx, note)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load note relations")
	}
	noteMessage, err := s.convertNoteFromStore(ctx, note, reactions, attachments, relations)
	if err != nil {
		if stderrors.Is(err, errNoteCreatorNotFound) {
			return nil, status.Errorf(codes.NotFound, "note creator not found")
		}
		return nil, errors.Wrap(err, "failed to convert note")
	}
	return noteMessage, nil
}

// UpdateNote updates an existing note.
func (s *APIService) UpdateNote(ctx context.Context, request *apipb.UpdateNoteRequest) (*apipb.Note, error) {
	if request.Note == nil {
		return nil, status.Errorf(codes.InvalidArgument, "note is required")
	}
	noteUID, err := ExtractNoteUIDFromName(request.Note.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}

	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note: %v", err)
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	update := &store.UpdateNote{
		ID: note.ID,
	}
	updatePaths := make(map[string]bool, len(request.UpdateMask.Paths))
	for _, path := range request.UpdateMask.Paths {
		updatePaths[path] = true
	}
	lifecycleOnly := updatePaths["space"]
	for path := range updatePaths {
		if path != "space" && path != "visibility" {
			lifecycleOnly = false
		}
	}
	nextSpaceID := note.SpaceID
	if updatePaths["space"] {
		if request.Note.Space == nil || request.Note.GetSpace() == "" {
			nextSpaceID = nil
			update.ClearSpace = true
		} else {
			target, err := s.resolveSpaceForNotePlacement(ctx, request.Note.GetSpace(), user)
			if err != nil {
				return nil, err
			}
			nextSpaceID = &target.ID
			update.SpaceID = &target.ID
			update.ClearSpace = false
		}
	}

	nextVisibility := note.Visibility
	if updatePaths["visibility"] {
		nextVisibility, err = validateUpdateNoteVisibility(request.Note.Visibility)
		if err != nil {
			return nil, err
		}
		update.Visibility = &nextVisibility
	}
	spaceChanged := !sameOptionalInt32(note.SpaceID, nextSpaceID)
	// A removed author receives a lifecycle exception only for a real withdraw
	// or move. Merely including the current Space in the mask must not smuggle
	// an audience-only mutation past membership checks.
	lifecycleOnly = lifecycleOnly && spaceChanged
	if nextVisibility == store.SpaceAudience && nextSpaceID == nil {
		return nil, status.Errorf(codes.InvalidArgument, "SPACE visibility requires a space")
	}
	if note.Visibility == store.SpaceAudience && spaceChanged && !updatePaths["visibility"] {
		return nil, status.Errorf(codes.InvalidArgument, "moving a note with SPACE visibility requires an explicit visibility update")
	}
	if note.Visibility == store.SpaceAudience && nextSpaceID == nil && nextVisibility == store.SpaceAudience {
		return nil, status.Errorf(codes.InvalidArgument, "unassigning a note with SPACE visibility requires a replacement visibility")
	}
	update.Policy = noteWritePolicy(user.ID, lifecycleOnly)
	previousContent := note.Content
	contentUpdated := false
	attachmentsUpdated := false
	relationsUpdated := false
	nextNote := *note
	if note.Payload != nil {
		nextNote.Payload = &storepb.NotePayload{}
		proto.Merge(nextNote.Payload, note.Payload)
	}

	for _, path := range request.UpdateMask.Paths {
		// Collaboration fields were validated together above so placement and
		// audience transitions cannot be observed independently.
		if path == "visibility" || path == "space" {
			continue
		}
		if path == "content" {
			contentUpdated = true
			contentLengthLimit, err := s.getContentLengthLimit(ctx)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to get content length limit")
			}
			if len(request.Note.Content) > contentLengthLimit {
				return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d bytes)", contentLengthLimit)
			}
			nextNote.Content = request.Note.Content
			if err := notepayload.RebuildNotePayload(ctx, &nextNote, s.MarkdownService); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to rebuild note payload: %v", err)
			}
			update.Content = &nextNote.Content
			update.Payload = nextNote.Payload
		} else if path == "pinned" {
			update.Pinned = &request.Note.Pinned
		} else if path == "state" {
			rowStatus := convertStateToStore(request.Note.State)
			update.RowStatus = &rowStatus
		} else if path == "create_time" {
			if request.Note.CreateTime == nil || !request.Note.CreateTime.IsValid() {
				return nil, status.Errorf(codes.InvalidArgument, "create_time is invalid")
			}
			createdTs := request.Note.CreateTime.AsTime().Unix()
			update.CreatedTs = &createdTs
		} else if path == "update_time" {
			updatedTsSec := time.Now().Unix()
			if request.Note.UpdateTime != nil {
				if !request.Note.UpdateTime.IsValid() {
					return nil, status.Errorf(codes.InvalidArgument, "update_time is invalid")
				}
				updatedTsSec = request.Note.UpdateTime.AsTime().Unix()
			}
			update.UpdatedTs = &updatedTsSec
		} else if path == "display_time" {
			return nil, status.Errorf(codes.InvalidArgument, "display_time is not supported")
		} else if path == "location" {
			if nextNote.Payload == nil {
				nextNote.Payload = &storepb.NotePayload{}
			}
			nextNote.Payload.Location = convertLocationToStore(request.Note.Location)
			update.Payload = nextNote.Payload
		} else if path == "attachments" {
			attachmentsUpdated = true
		} else if path == "relations" {
			relationsUpdated = true
		} else {
			return nil, status.Errorf(codes.InvalidArgument, "invalid update path: %s", path)
		}
	}

	var preparedAttachments *preparedNoteAttachments
	if attachmentsUpdated {
		preparedAttachments, err = s.prepareNoteAttachments(ctx, user, note, request.Note.Attachments)
		if err != nil {
			return nil, err
		}
	}
	var preparedRelations []*store.NoteRelation
	if relationsUpdated {
		preparedRelations, err = s.prepareNoteRelations(ctx, note, request.Note.Relations)
		if err != nil {
			return nil, err
		}
	}
	var requiredAttachmentIDs []int32
	if contentUpdated || attachmentsUpdated {
		var finalAttachments []*store.Attachment
		if preparedAttachments != nil {
			finalAttachments = preparedAttachments.normalized
		} else {
			finalAttachments, err = s.Store.ListAttachments(ctx, &store.FindAttachment{NoteID: &note.ID})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to list attachments")
			}
		}
		requiredAttachmentIDs, err = s.resolveNoteAttachmentReferences(nextNote.Content, finalAttachments)
		if err != nil {
			return nil, err
		}
	}

	if contentUpdated || attachmentsUpdated || relationsUpdated {
		var relations *[]*store.NoteRelation
		if relationsUpdated {
			relations = &preparedRelations
		}
		if err := s.applyNoteMutation(ctx, note, preparedAttachments, update, requiredAttachmentIDs, relations); err != nil {
			return nil, err
		}
	} else if err = s.Store.UpdateNote(ctx, update); err != nil {
		return nil, mapNoteWriteError(err, "failed to update note")
	}

	note, commentContext, noteMessage, err := s.buildUpdatedNoteState(ctx, note.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build updated note state")
	}
	if contentUpdated {
		s.dispatchNoteMentionNotificationsBestEffort(ctx, note, commentContext, previousContent)
	}
	s.dispatchNoteUpdatedSideEffects(ctx, noteMessage)

	return noteMessage, nil
}

func (s *APIService) DeleteNote(ctx context.Context, request *apipb.DeleteNoteRequest) (*emptypb.Empty, error) {
	noteUID, err := ExtractNoteUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{
		UID: &noteUID,
	})
	if err != nil {
		return nil, err
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	var deletedNoteMessage *apipb.Note
	// Deletion is a narrow lifecycle capability and may remain available after
	// the author loses read access to a note with the SPACE audience. Only
	// build a content-bearing webhook payload when the actor can still read the
	// note immediately before deletion.
	if s.checkNoteReadAccess(ctx, note) == nil {
		reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{NoteID: &note.ID})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list reactions")
		}
		attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteID: &note.ID})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list attachments")
		}
		deleteRelations, _ := s.loadNoteRelations(ctx, note)
		if noteMessage, err := s.convertNoteFromStore(ctx, note, reactions, attachments, deleteRelations); err == nil {
			deletedNoteMessage = noteMessage
		}
	}

	deleteResult, err := s.Store.DeleteNoteWithPolicy(ctx, &store.DeleteNoteWithPolicy{NoteID: note.ID, ActorUserID: user.ID})
	if err != nil {
		switch {
		case stderrors.Is(err, store.ErrNotePermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		case stderrors.Is(err, store.ErrNoteMutationConflict):
			return nil, status.Error(codes.FailedPrecondition, "note changed or no longer exists")
		default:
			return nil, status.Errorf(codes.Internal, "failed to delete note: %v", err)
		}
	}
	if deleteResult == nil {
		return nil, status.Error(codes.Internal, "note was deleted without authorization state")
	}

	s.SSEHub.publishNoteChanged()
	if deletedNoteMessage != nil && deleteResult.ActorCanRead {
		if err := s.DispatchNoteDeletedWebhook(ctx, deletedNoteMessage); err != nil {
			slog.Warn("Failed to dispatch note deleted webhook", slog.Any("err", err))
		}
	}
	if err := s.cleanupDeletedAttachmentStorage(ctx, deleteResult.Attachments); err != nil {
		return nil, status.Errorf(codes.Internal, "note was deleted but attachment storage cleanup failed: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *APIService) getContentLengthLimit(ctx context.Context) (int, error) {
	instanceNoteRelatedSetting, err := s.Store.GetInstanceNoteRelatedSetting(ctx)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "failed to get instance note related setting")
	}
	return int(instanceNoteRelatedSetting.ContentLengthLimit), nil
}

// DispatchNoteCreatedWebhook dispatches webhook when note is created.

package api

import (
	"context"
	stderrors "errors"
	"log/slog"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// CreateNoteComment creates a comment on an existing note.
func (s *APIService) CreateNoteComment(ctx context.Context, request *apipb.CreateNoteCommentRequest) (*apipb.Note, error) {
	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	relatedNote, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if relatedNote == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	// Check note visibility before allowing comment.
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := requireNoteWriter(user); err != nil {
		return nil, err
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}
	if err := s.checkNoteReadAccess(ctx, relatedNote); err != nil {
		return nil, err
	}
	if relatedNote.RowStatus != store.Normal {
		return nil, status.Errorf(codes.FailedPrecondition, "cannot comment on an archived note")
	}
	if request.Comment == nil {
		return nil, status.Errorf(codes.InvalidArgument, "comment is required")
	}

	noteUID, err = ValidateAndGenerateUID(request.CommentId)
	if err != nil {
		return nil, err
	}
	if noteUID == relatedNote.UID {
		return nil, status.Error(codes.InvalidArgument, "a note cannot comment on itself")
	}
	prepared, err := s.prepareNoteCreate(ctx, user, request.Comment, noteUID)
	if err != nil {
		return nil, err
	}
	// A comment is a note row, so it counts against the note limit too.
	if err := s.checkNoteQuota(ctx, user); err != nil {
		return nil, err
	}
	if err := s.createNoteWithMutation(ctx, user, prepared.note, &relatedNote.ID, prepared.attachments, prepared.requiredAttachmentIDs, prepared.referenceRelations); err != nil {
		return nil, mapNoteCreateError(err, noteUID, "failed to create note comment")
	}
	note := prepared.note
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteID: &note.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load note comment attachments")
	}
	relations, err := s.loadNoteRelations(ctx, note)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load note relations")
	}
	noteComment, err := s.convertNoteFromStore(ctx, note, nil, attachments, relations)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert note comment")
	}

	s.createNoteCommentNotification(ctx, user.ID, note, relatedNote)

	if err := s.DispatchNoteCommentCreatedWebhook(ctx, note, relatedNote, relatedNote.CreatorID); err != nil {
		slog.Warn("Failed to dispatch note comment created webhook", slog.Any("err", err))
	}

	s.dispatchNoteMentionNotificationsBestEffort(ctx, note, relatedNote, "")

	s.SSEHub.publishNoteChanged()

	return noteComment, nil
}

// createNoteCommentNotification records the comment in the related note
// creator's inbox. The comment itself is already persisted, so a failure here
// only loses the notification and is logged instead of failing the request;
// failing it would make clients retry and create a duplicate comment.
func (s *APIService) createNoteCommentNotification(ctx context.Context, creatorID int32, note *store.Note, relatedNote *store.Note) {
	if creatorID == relatedNote.CreatorID {
		return
	}
	relatedCreator, err := s.Store.GetUser(ctx, &store.FindUser{ID: &relatedNote.CreatorID})
	if err != nil {
		slog.Warn("Failed to get related note creator for comment notification",
			slog.Any("err", err),
			slog.Int64("note_id", int64(note.ID)))
		return
	}
	if !s.canUserAccessMentionNotes(ctx, relatedCreator, note, relatedNote) {
		return
	}
	if _, err := s.createInboxWithEmailNotification(ctx, &store.Inbox{
		SenderID:   creatorID,
		ReceiverID: relatedNote.CreatorID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_COMMENT,
			Payload: &storepb.InboxMessage_NoteComment{
				NoteComment: &storepb.InboxMessage_NoteCommentPayload{
					NoteId:        note.ID,
					RelatedNoteId: relatedNote.ID,
				},
			},
		},
	}); err != nil {
		slog.Warn("Failed to create note comment notification",
			slog.Any("err", err),
			slog.Int64("note_id", int64(note.ID)),
			slog.Int64("receiver_id", int64(relatedNote.CreatorID)))
	}
}

func (s *APIService) ListNoteComments(ctx context.Context, request *apipb.ListNoteCommentsRequest) (*apipb.ListNoteCommentsResponse, error) {
	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}
	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}
	accessScope, _, err := s.resolveNoteAccessScope(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
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
	limitPlusOne := limit + 1
	normal := store.Normal
	notes, err := s.Store.ListNotes(ctx, &store.FindNote{
		CommentContextNoteID: &note.ID,
		RowStatus:            &normal,
		Access:               accessScope,
		Limit:                &limitPlusOne,
		Offset:               &offset,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list note comments")
	}

	nextPageToken := ""
	if len(notes) == limitPlusOne {
		notes = notes[:limit]
		nextPageToken, err = getPageToken(limit, offset+limit)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token, error: %v", err)
		}
	}

	if len(notes) == 0 {
		response := &apipb.ListNoteCommentsResponse{
			Comments:      []*apipb.Note{},
			NextPageToken: nextPageToken,
		}
		return response, nil
	}

	noteIDs := make([]int32, 0, len(notes))

	for _, note := range notes {
		noteIDs = append(noteIDs, note.ID)
	}
	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{NoteIDList: noteIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}

	noteReactionsMap := make(map[int32][]*store.Reaction)
	for _, reaction := range reactions {
		noteReactionsMap[reaction.NoteID] = append(noteReactionsMap[reaction.NoteID], reaction)
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteIDList: noteIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}
	attachmentMap := make(map[int32][]*store.Attachment)
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
	var notesResponse []*apipb.Note
	for _, m := range notes {
		reactions := noteReactionsMap[m.ID]
		attachments := attachmentMap[m.ID]
		relations := relationMap[m.ID]

		noteMessage, err := s.convertNoteFromStoreWithCreators(ctx, m, reactions, attachments, relations, creatorMap)
		if err != nil {
			if stderrors.Is(err, errNoteCreatorNotFound) {
				slog.Warn("Skipping note comment with missing creator",
					slog.Int64("note_id", int64(m.ID)),
					slog.String("note_uid", m.UID),
					slog.Int64("creator_id", int64(m.CreatorID)),
					slog.String("parent", request.Parent),
				)
				continue
			}
			return nil, errors.Wrap(err, "failed to convert note")
		}
		notesResponse = append(notesResponse, noteMessage)
	}

	response := &apipb.ListNoteCommentsResponse{
		Comments:      notesResponse,
		NextPageToken: nextPageToken,
	}
	return response, nil
}

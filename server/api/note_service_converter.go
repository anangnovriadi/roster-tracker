package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

var (
	errNoteCreatorNotFound     = stderrors.New("note creator not found")
	errReactionCreatorNotFound = stderrors.New("reaction creator not found")
)

func (s *APIService) convertNoteFromStore(ctx context.Context, note *store.Note, reactions []*store.Reaction, attachments []*store.Attachment, relations []*apipb.NoteRelation) (*apipb.Note, error) {
	creatorMap, err := s.listUsersByID(ctx, []int32{note.CreatorID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list note creators")
	}
	return s.convertNoteFromStoreWithCreators(ctx, note, reactions, attachments, relations, creatorMap)
}

func (s *APIService) convertNoteFromStoreWithCreators(ctx context.Context, note *store.Note, reactions []*store.Reaction, attachments []*store.Attachment, relations []*apipb.NoteRelation, creatorMap map[int32]*store.User) (*apipb.Note, error) {
	name := buildNoteName(note.UID)
	creator := creatorMap[note.CreatorID]
	if creator == nil {
		return nil, errNoteCreatorNotFound
	}
	noteMessage := &apipb.Note{
		Name:       name,
		State:      convertStateFromStore(note.RowStatus),
		Creator:    BuildUserName(creator.Username),
		CreateTime: timestamppb.New(time.Unix(note.CreatedTs, 0)),
		UpdateTime: timestamppb.New(time.Unix(note.UpdatedTs, 0)),
		Content:    note.Content,
		Visibility: convertVisibilityFromStore(note.Visibility),
		Pinned:     note.Pinned,
	}
	// An assigned note needs a read context for its placement projection; an
	// unassigned note needs none.
	var readContext access.NoteReadContext
	if note.SpaceID != nil {
		resolved, err := s.buildNoteReadContext(ctx, note, nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve note access")
		}
		readContext = resolved
	}
	if err := s.projectNoteCollaborationContext(ctx, note, noteMessage, readContext); err != nil {
		return nil, errors.Wrap(err, "failed to project note collaboration context")
	}
	if note.Payload != nil {
		noteMessage.Tags = note.Payload.Tags
		noteMessage.Property = convertNotePropertyFromStore(note.Payload.Property)
		noteMessage.Location = convertLocationFromStore(note.Payload.Location)
	}

	// Parent identity is part of a readable comment's context. It grants no
	// access to the parent; clients resolve it independently and handle denial.
	if note.ParentUID != nil {
		parentName := buildNoteName(*note.ParentUID)
		noteMessage.Parent = &parentName
	}

	// Reactions have no independent audience and are readable whenever this
	// note is readable. Conversion is reached only after note authorization.
	reactionMessages, err := s.convertReactionsFromStoreWithCreators(ctx, reactions, creatorMap, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert reactions")
	}
	noteMessage.Reactions = reactionMessages

	if relations != nil {
		noteMessage.Relations = relations
	} else {
		noteMessage.Relations = []*apipb.NoteRelation{}
	}

	noteMessage.Attachments = []*apipb.Attachment{}
	for _, attachment := range attachments {
		attachmentResponse := convertAttachmentFromStore(attachment)
		noteMessage.Attachments = append(noteMessage.Attachments, attachmentResponse)
	}

	snippet, err := s.getNoteContentSnippet(note.Content)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get note content snippet")
	}
	noteMessage.Snippet = snippet

	return noteMessage, nil
}

func (s *APIService) projectNoteCollaborationContext(ctx context.Context, note *store.Note, message *apipb.Note, readContext access.NoteReadContext) error {
	if note.SpaceID == nil {
		return nil
	}
	if !readContext.SpaceValid {
		// Placement is not an additional read gate for PRIVATE, PROTECTED, or
		// PUBLIC. Omit a dangling placement rather than turning it into a second
		// audience restriction. SPACE already failed closed in the read policy.
		if note.Visibility == store.SpaceAudience {
			return errors.New("note has invalid space placement")
		}
		return nil
	}
	// Placement is visible to the author, to active members, and to an
	// instance administrator; a non-member reading an assigned PUBLIC note
	// learns nothing about its Space.
	viewer := readContext.Viewer
	if viewer == nil || (viewer.ID != note.CreatorID && !readContext.ViewerSpaceMember && !access.IsInstanceAdmin(viewer)) {
		return nil
	}
	space, err := s.Store.GetSpace(ctx, &store.FindSpace{ID: note.SpaceID})
	if err != nil {
		return err
	}
	if space == nil {
		if note.Visibility == store.SpaceAudience {
			return errors.New("note has invalid space placement")
		}
		return nil
	}
	spaceName := buildSpaceName(space.UID)
	message.Space = &spaceName
	return nil
}

func (s *APIService) listUsersByIDWithExisting(ctx context.Context, userIDs []int32, existing map[int32]*store.User) (map[int32]*store.User, error) {
	usersByID := make(map[int32]*store.User, len(existing)+len(userIDs))
	for userID, user := range existing {
		if user != nil {
			usersByID[userID] = user
		}
	}

	missingUserIDs := make([]int32, 0, len(userIDs))
	seenMissingUserIDs := make(map[int32]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if _, ok := usersByID[userID]; ok {
			continue
		}
		if _, ok := seenMissingUserIDs[userID]; ok {
			continue
		}
		seenMissingUserIDs[userID] = struct{}{}
		missingUserIDs = append(missingUserIDs, userID)
	}

	if len(missingUserIDs) == 0 {
		return usersByID, nil
	}

	missingUsersByID, err := s.listUsersByID(ctx, missingUserIDs)
	if err != nil {
		return nil, err
	}
	for userID, user := range missingUsersByID {
		if user != nil {
			usersByID[userID] = user
		}
	}
	return usersByID, nil
}

func (s *APIService) convertReactionsFromStoreWithCreators(ctx context.Context, reactions []*store.Reaction, creatorMap map[int32]*store.User, noteName string) ([]*apipb.Reaction, error) {
	if len(reactions) == 0 {
		return []*apipb.Reaction{}, nil
	}

	creatorIDs := make([]int32, 0, len(reactions))
	for _, reaction := range reactions {
		creatorIDs = append(creatorIDs, reaction.CreatorID)
	}
	creatorsByID, err := s.listUsersByIDWithExisting(ctx, creatorIDs, creatorMap)
	if err != nil {
		return nil, err
	}

	reactionMessages := make([]*apipb.Reaction, 0, len(reactions))
	for _, reaction := range reactions {
		reactionMessage, err := convertReactionFromStoreWithCreators(reaction, creatorsByID, noteName)
		if err != nil {
			if stderrors.Is(err, errReactionCreatorNotFound) {
				slog.Warn("Skipping reaction with missing creator",
					slog.Int64("reaction_id", int64(reaction.ID)),
					slog.Int64("creator_id", int64(reaction.CreatorID)),
					slog.Int64("note_id", int64(reaction.NoteID)),
				)
				continue
			}
			return nil, err
		}
		reactionMessages = append(reactionMessages, reactionMessage)
	}
	return reactionMessages, nil
}

func convertReactionFromStoreWithCreators(reaction *store.Reaction, creatorsByID map[int32]*store.User, noteName string) (*apipb.Reaction, error) {
	creator := creatorsByID[reaction.CreatorID]
	if creator == nil {
		return nil, errReactionCreatorNotFound
	}

	reactionUID := fmt.Sprintf("%d", reaction.ID)
	return &apipb.Reaction{
		Name:         fmt.Sprintf("%s/%s%s", noteName, ReactionNamePrefix, reactionUID),
		Creator:      BuildUserName(creator.Username),
		ReactionType: reaction.ReactionType,
		CreateTime:   timestamppb.New(time.Unix(reaction.CreatedTs, 0)),
	}, nil
}

// batchConvertNoteRelations batch-loads relations for a list of notes and returns
// a map from note ID to its converted relations. This avoids N+1 queries when listing notes.
func (s *APIService) batchConvertNoteRelations(ctx context.Context, notes []*store.Note, includeSnippets bool) (map[int32][]*apipb.NoteRelation, error) {
	if len(notes) == 0 {
		return map[int32][]*apipb.NoteRelation{}, nil
	}

	accessScope, _, err := s.resolveNoteAccessScope(ctx)
	if err != nil {
		return nil, err
	}

	noteIDs := make([]int32, len(notes))
	noteIDSet := make(map[int32]bool, len(notes))
	for i, m := range notes {
		noteIDs[i] = m.ID
		noteIDSet[m.ID] = true
	}

	outgoingRelations, err := s.Store.ListNoteRelations(ctx, &store.FindNoteRelation{
		SourceNoteIDList: noteIDs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch list outgoing note relations")
	}
	incomingRelations, err := s.Store.ListNoteRelations(ctx, &store.FindNoteRelation{
		RelatedNoteIDList: noteIDs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to batch list incoming note relations")
	}
	allRelations := mergeNoteRelations(outgoingRelations, incomingRelations)

	// Collect all note IDs referenced in relations that we need to resolve.
	neededIDs := make(map[int32]bool)
	for _, r := range allRelations {
		neededIDs[r.NoteID] = true
		neededIDs[r.RelatedNoteID] = true
	}

	// Build ID→UID map from the notes we already have.
	noteIDToUID := make(map[int32]string, len(notes))
	noteIDToSnippet := make(map[int32]string, len(notes))
	for _, m := range notes {
		noteIDToUID[m.ID] = m.UID
		if includeSnippets {
			snippet, err := s.getNoteContentSnippet(m.Content)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get note content snippet")
			}
			noteIDToSnippet[m.ID] = snippet
		}
		delete(neededIDs, m.ID)
	}

	// Batch fetch any additional notes referenced by relations that we don't already have.
	if len(neededIDs) > 0 {
		extraIDs := make([]int32, 0, len(neededIDs))
		for id := range neededIDs {
			extraIDs = append(extraIDs, id)
		}
		extraFind := &store.FindNote{IDList: extraIDs, ExcludeContent: !includeSnippets, Access: accessScope}
		extraNotes, err := s.Store.ListNotes(ctx, extraFind)
		if err != nil {
			return nil, errors.Wrap(err, "failed to batch fetch related notes")
		}
		for _, m := range extraNotes {
			noteIDToUID[m.ID] = m.UID
			if includeSnippets {
				snippet, err := s.getNoteContentSnippet(m.Content)
				if err != nil {
					return nil, errors.Wrap(err, "failed to get related note content snippet")
				}
				noteIDToSnippet[m.ID] = snippet
			}
		}
	}

	// Build the result map: note ID → its relations (both directions).
	result := make(map[int32][]*apipb.NoteRelation, len(notes))
	for _, r := range allRelations {
		noteUID, ok1 := noteIDToUID[r.NoteID]
		relatedUID, ok2 := noteIDToUID[r.RelatedNoteID]
		if !ok1 || !ok2 {
			continue
		}

		relation := &apipb.NoteRelation{
			Note: &apipb.NoteRelation_NoteRef{
				Name:    fmt.Sprintf("%s%s", NoteNamePrefix, noteUID),
				Snippet: noteIDToSnippet[r.NoteID],
			},
			RelatedNote: &apipb.NoteRelation_NoteRef{
				Name:    fmt.Sprintf("%s%s", NoteNamePrefix, relatedUID),
				Snippet: noteIDToSnippet[r.RelatedNoteID],
			},
			Type: convertNoteRelationTypeFromStore(r.Type),
		}

		// Add to the note that owns this relation (both directions).
		if noteIDSet[r.NoteID] {
			result[r.NoteID] = append(result[r.NoteID], relation)
		}
		if noteIDSet[r.RelatedNoteID] {
			result[r.RelatedNoteID] = append(result[r.RelatedNoteID], relation)
		}
	}

	return result, nil
}

// loadNoteRelations loads relations for a single note and converts them to API format.
func (s *APIService) loadNoteRelations(ctx context.Context, note *store.Note) ([]*apipb.NoteRelation, error) {
	relationMap, err := s.batchConvertNoteRelations(ctx, []*store.Note{note}, true)
	if err != nil {
		return nil, err
	}
	return relationMap[note.ID], nil
}

func mergeNoteRelations(groups ...[]*store.NoteRelation) []*store.NoteRelation {
	seen := make(map[string]struct{})
	merged := make([]*store.NoteRelation, 0)
	for _, relations := range groups {
		for _, relation := range relations {
			key := fmt.Sprintf("%d:%d:%s", relation.NoteID, relation.RelatedNoteID, relation.Type)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, relation)
		}
	}
	return merged
}

func convertNotePropertyFromStore(property *storepb.NotePayload_Property) *apipb.Note_Property {
	if property == nil {
		return nil
	}
	return &apipb.Note_Property{
		HasLink:            property.HasLink,
		HasTaskList:        property.HasTaskList,
		HasCode:            property.HasCode,
		HasIncompleteTasks: property.HasIncompleteTasks,
		Title:              property.Title,
	}
}

func convertLocationFromStore(location *storepb.NotePayload_Location) *apipb.Location {
	if location == nil {
		return nil
	}
	return &apipb.Location{
		Placeholder: location.Placeholder,
		Latitude:    location.Latitude,
		Longitude:   location.Longitude,
	}
}

func convertLocationToStore(location *apipb.Location) *storepb.NotePayload_Location {
	if location == nil {
		return nil
	}
	return &storepb.NotePayload_Location{
		Placeholder: location.Placeholder,
		Latitude:    location.Latitude,
		Longitude:   location.Longitude,
	}
}

func convertVisibilityFromStore(visibility store.Visibility) apipb.Visibility {
	switch visibility {
	case store.Private:
		return apipb.Visibility_PRIVATE
	case store.Protected:
		return apipb.Visibility_PROTECTED
	case store.Public:
		return apipb.Visibility_PUBLIC
	case store.SpaceAudience:
		return apipb.Visibility_SPACE
	default:
		return apipb.Visibility_VISIBILITY_UNSPECIFIED
	}
}

func validateCreateNoteVisibility(visibility apipb.Visibility) (store.Visibility, error) {
	switch visibility {
	case apipb.Visibility_VISIBILITY_UNSPECIFIED, apipb.Visibility_PRIVATE:
		return store.Private, nil
	case apipb.Visibility_PROTECTED:
		return store.Protected, nil
	case apipb.Visibility_PUBLIC:
		return store.Public, nil
	case apipb.Visibility_SPACE:
		return store.SpaceAudience, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "invalid note visibility")
	}
}

func validateUpdateNoteVisibility(visibility apipb.Visibility) (store.Visibility, error) {
	if visibility == apipb.Visibility_VISIBILITY_UNSPECIFIED {
		return "", status.Errorf(codes.InvalidArgument, "visibility must be specified")
	}
	return validateCreateNoteVisibility(visibility)
}

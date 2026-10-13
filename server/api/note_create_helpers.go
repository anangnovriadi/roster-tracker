package api

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/core/notepayload"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func mapNoteCreateError(err error, uid, operation string) error {
	errMessage := err.Error()
	if strings.Contains(errMessage, "UNIQUE constraint failed") ||
		strings.Contains(errMessage, "duplicate key") ||
		strings.Contains(errMessage, "Duplicate entry") {
		return status.Errorf(codes.AlreadyExists, "note with ID %q already exists", uid)
	}
	return mapNoteWriteError(err, operation)
}

type preparedNoteCreate struct {
	note                  *store.Note
	attachments           *preparedNoteAttachments
	requiredAttachmentIDs []int32
	referenceRelations    []*store.NoteRelation
}

// prepareNoteCreate validates and prepares the fields shared by top-level
// note and comment creation. It does not write database or external state.
func (s *APIService) prepareNoteCreate(ctx context.Context, user *store.User, input *apipb.Note, uid string) (*preparedNoteCreate, error) {
	if user == nil || input == nil {
		return nil, status.Error(codes.InvalidArgument, "note is required")
	}
	visibility, err := validateCreateNoteVisibility(input.Visibility)
	if err != nil {
		return nil, err
	}
	note := &store.Note{
		UID:        uid,
		CreatorID:  user.ID,
		Content:    input.Content,
		Visibility: visibility,
	}
	if input.Space != nil && input.GetSpace() != "" {
		space, err := s.resolveSpaceForNotePlacement(ctx, input.GetSpace(), user)
		if err != nil {
			return nil, err
		}
		note.SpaceID = &space.ID
	}
	if note.Visibility == store.SpaceAudience && note.SpaceID == nil {
		return nil, status.Error(codes.InvalidArgument, "SPACE visibility requires a space")
	}
	if input.CreateTime != nil && input.CreateTime.IsValid() {
		note.CreatedTs = input.CreateTime.AsTime().Unix()
	}
	if input.UpdateTime != nil && input.UpdateTime.IsValid() {
		note.UpdatedTs = input.UpdateTime.AsTime().Unix()
	}

	contentLengthLimit, err := s.getContentLengthLimit(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get content length limit")
	}
	if len(note.Content) > contentLengthLimit {
		return nil, status.Errorf(codes.InvalidArgument, "content too long (max %d bytes)", contentLengthLimit)
	}
	if err := notepayload.RebuildNotePayload(ctx, note, s.MarkdownService); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rebuild note payload: %v", err)
	}
	if input.Location != nil {
		note.Payload.Location = convertLocationToStore(input.Location)
	}

	attachments, err := s.prepareNoteAttachments(ctx, user, note, input.Attachments)
	if err != nil {
		return nil, err
	}
	requiredAttachmentIDs, err := s.resolveNoteAttachmentReferences(note.Content, attachments.normalized)
	if err != nil {
		return nil, err
	}
	relations, err := s.prepareNoteRelations(ctx, note, input.Relations)
	if err != nil {
		return nil, err
	}
	return &preparedNoteCreate{
		note:                  note,
		attachments:           attachments,
		requiredAttachmentIDs: requiredAttachmentIDs,
		referenceRelations:    relations,
	}, nil
}

package api

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) buildUpdatedNoteState(ctx context.Context, noteID int32) (*store.Note, *store.Note, *apipb.Note, error) {
	note, err := s.Store.GetNote(ctx, &store.FindNote{ID: &noteID})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get note")
	}
	if note == nil {
		return nil, nil, nil, errors.New("note not found")
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to list reactions")
	}
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to list attachments")
	}
	relations, err := s.loadNoteRelations(ctx, note)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to load note relations")
	}
	noteMessage, err := s.convertNoteFromStore(ctx, note, reactions, attachments, relations)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to convert note")
	}

	var commentContext *store.Note
	if note.ParentUID != nil {
		commentContext, err = s.Store.GetNote(ctx, &store.FindNote{UID: note.ParentUID})
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to get comment context")
		}
	}

	return note, commentContext, noteMessage, nil
}

func (s *APIService) dispatchNoteUpdatedSideEffects(
	ctx context.Context,
	noteMessage *apipb.Note,
) {
	if err := s.DispatchNoteUpdatedWebhook(ctx, noteMessage); err != nil {
		slog.Warn("Failed to dispatch note updated webhook", slog.Any("err", err))
	}

	s.SSEHub.publishNoteChanged()
}

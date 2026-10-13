package notepayload

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/markdown"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

type Runner struct {
	Store           *store.Store
	MarkdownService markdown.Service
}

func NewRunner(store *store.Store, markdownService markdown.Service) *Runner {
	return &Runner{
		Store:           store,
		MarkdownService: markdownService,
	}
}

// RunOnce rebuilds the payload of all notes.
func (r *Runner) RunOnce(ctx context.Context) {
	// Process notes in batches to avoid loading all notes into memory at once
	const batchSize = 100
	offset := 0
	processed := 0

	for {
		limit := batchSize
		notes, err := r.Store.ListNotes(ctx, &store.FindNote{
			Limit:  &limit,
			Offset: &offset,
		})
		if err != nil {
			slog.Error("failed to list notes", "err", err)
			return
		}

		// Break if no more notes
		if len(notes) == 0 {
			break
		}

		// Process batch
		batchSuccessCount := 0
		for _, note := range notes {
			if err := RebuildNotePayload(ctx, note, r.MarkdownService); err != nil {
				slog.Error("failed to rebuild note payload", "err", err, "noteID", note.ID)
				continue
			}
			if err := r.Store.UpdateNote(ctx, &store.UpdateNote{
				ID:      note.ID,
				Payload: note.Payload,
			}); err != nil {
				slog.Error("failed to update note", "err", err, "noteID", note.ID)
				continue
			}
			batchSuccessCount++
		}

		processed += len(notes)
		slog.Info("Processed note batch", "batchSize", len(notes), "successCount", batchSuccessCount, "totalProcessed", processed)

		// Move to next batch
		offset += len(notes)
	}
}

func RebuildNotePayload(_ context.Context, note *store.Note, markdownService markdown.Service) error {
	if note.Payload == nil {
		note.Payload = &storepb.NotePayload{}
	}

	// Use goldmark service to extract all metadata in a single pass (more efficient)
	data, err := markdownService.ExtractAll([]byte(note.Content))
	if err != nil {
		return errors.Wrap(err, "failed to extract markdown metadata")
	}

	note.Payload.Tags = data.Tags
	note.Payload.Property = data.Property
	return nil
}

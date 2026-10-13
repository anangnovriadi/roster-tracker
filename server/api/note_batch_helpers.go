package api

import (
	"context"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) listNotesByID(ctx context.Context, noteIDs []int32) (map[int32]*store.Note, error) {
	if len(noteIDs) == 0 {
		return map[int32]*store.Note{}, nil
	}

	uniqueNoteIDs := make([]int32, 0, len(noteIDs))
	seenNoteIDs := make(map[int32]struct{}, len(noteIDs))
	for _, noteID := range noteIDs {
		if _, seen := seenNoteIDs[noteID]; seen {
			continue
		}
		seenNoteIDs[noteID] = struct{}{}
		uniqueNoteIDs = append(uniqueNoteIDs, noteID)
	}

	notes, err := s.Store.ListNotes(ctx, &store.FindNote{IDList: uniqueNoteIDs})
	if err != nil {
		return nil, err
	}

	notesByID := make(map[int32]*store.Note, len(notes))
	for _, note := range notes {
		notesByID[note.ID] = note
	}
	return notesByID, nil
}

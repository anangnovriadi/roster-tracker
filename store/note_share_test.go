package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteShareRejectsInvalidInputBeforeWriting(t *testing.T) {
	ts := &store.Store{} // Invalid input must be rejected before using a driver.
	for _, test := range []struct {
		name   string
		delete *store.DeleteNoteShare
	}{
		{"nil", nil},
		{"no selector", &store.DeleteNoteShare{}},
		{"note only", &store.DeleteNoteShare{NoteID: new(int32(1))}},
		{"missing actor", &store.DeleteNoteShare{ID: new(int32(1)), NoteID: new(int32(1)), Policy: &store.NoteWritePolicy{}}},
		{"missing note", &store.DeleteNoteShare{ID: new(int32(1)), Policy: &store.NoteWritePolicy{ActorUserID: 1}}},
		{"invalid note", &store.DeleteNoteShare{ID: new(int32(1)), NoteID: new(int32(0)), Policy: &store.NoteWritePolicy{ActorUserID: 1}}},
		{"conflicting actions", &store.DeleteNoteShare{ID: new(int32(1)), NoteID: new(int32(1)), Policy: &store.NoteWritePolicy{
			ActorUserID: 1, LifecycleOnly: true, CreatingShare: true,
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ts.DeleteNoteShare(context.Background(), test.delete))
		})
	}
	_, err := ts.CreateNoteShare(context.Background(), nil)
	require.Error(t, err)
	_, err = ts.CreateNoteShare(context.Background(), &store.NoteShare{Policy: &store.NoteWritePolicy{}})
	require.Error(t, err)
}

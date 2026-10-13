package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteTimestampUpdates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	originalCreate, originalUpdate := int64(1577836800), int64(1577840400)
	created, updated := originalCreate-86400, originalUpdate-86400

	for _, tc := range []struct {
		name       string
		patch      *store.UpdateNote
		wantCreate int64
		wantUpdate int64
	}{
		{"creation-only", &store.UpdateNote{CreatedTs: &created}, created, originalUpdate},
		{"modification-only", &store.UpdateNote{UpdatedTs: &updated}, originalCreate, updated},
		{"both", &store.UpdateNote{CreatedTs: &created, UpdatedTs: &updated}, created, updated},
		{"unrelated-update", &store.UpdateNote{Content: new("changed")}, originalCreate, originalUpdate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			note, err := ts.CreateNote(ctx, &store.Note{
				UID: tc.name, CreatorID: user.ID, Content: "original", Visibility: store.Private,
				CreatedTs: originalCreate, UpdatedTs: originalUpdate,
			})
			require.NoError(t, err)
			tc.patch.ID = note.ID
			require.NoError(t, ts.UpdateNote(ctx, tc.patch))
			got, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tc.wantCreate, got.CreatedTs)
			require.Equal(t, tc.wantUpdate, got.UpdatedTs)
			if tc.patch.Content != nil {
				require.Equal(t, *tc.patch.Content, got.Content)
			}
		})
	}
}

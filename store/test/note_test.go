package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
)

func TestNoteStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	noteCreate := &store.Note{
		UID:        "test-resource-name",
		CreatorID:  user.ID,
		Content:    "test_content",
		Visibility: store.Public,
	}
	note, err := ts.CreateNote(ctx, noteCreate)
	require.NoError(t, err)
	require.Equal(t, noteCreate.Content, note.Content)
	notePatchContent := "test_content_2"
	notePatch := &store.UpdateNote{
		ID:      note.ID,
		Content: &notePatchContent,
	}
	err = ts.UpdateNote(ctx, notePatch)
	require.NoError(t, err)
	note, err = ts.GetNote(ctx, &store.FindNote{
		ID: &note.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, note)
	noteList, err := ts.ListNotes(ctx, &store.FindNote{
		CreatorID: &user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(noteList))
	require.Equal(t, note, noteList[0])
	err = ts.DeleteNote(ctx, &store.DeleteNote{
		ID: note.ID,
	})
	require.NoError(t, err)
	noteList, err = ts.ListNotes(ctx, &store.FindNote{
		CreatorID: &user.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(noteList))

	noteList, err = ts.ListNotes(ctx, &store.FindNote{
		CreatorID:      &user.ID,
		VisibilityList: []store.Visibility{store.Public},
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(noteList))
	ts.Close()
}

func TestNoteListByTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	noteCreate := &store.Note{
		UID:        "test-resource-name",
		CreatorID:  user.ID,
		Content:    "test_content",
		Visibility: store.Public,
		Payload: &storepb.NotePayload{
			Tags: []string{"test_tag"},
		},
	}
	note, err := ts.CreateNote(ctx, noteCreate)
	require.NoError(t, err)
	require.Equal(t, noteCreate.Content, note.Content)
	note, err = ts.GetNote(ctx, &store.FindNote{
		ID: &note.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, note)

	noteList, err := ts.ListNotes(ctx, &store.FindNote{
		Filters: []string{"tag in [\"test_tag\"]"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(noteList))
	require.Equal(t, note, noteList[0])
	ts.Close()
}

func TestDeleteNoteStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	noteCreate := &store.Note{
		UID:        "test-resource-name",
		CreatorID:  user.ID,
		Content:    "test_content",
		Visibility: store.Public,
	}
	note, err := ts.CreateNote(ctx, noteCreate)
	require.NoError(t, err)
	require.Equal(t, noteCreate.Content, note.Content)
	err = ts.DeleteNote(ctx, &store.DeleteNote{
		ID: note.ID,
	})
	require.NoError(t, err)
	ts.Close()
}

func TestNoteGetByID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "test-note-1",
		CreatorID:  user.ID,
		Content:    "test content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Get by ID
	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, note.ID, found.ID)
	require.Equal(t, note.Content, found.Content)

	// Get non-existent
	nonExistentID := int32(99999)
	notFound, err := ts.GetNote(ctx, &store.FindNote{ID: &nonExistentID})
	require.NoError(t, err)
	require.Nil(t, notFound)

	ts.Close()
}

func TestNoteGetByUID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	uid := "unique-note-uid"
	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        uid,
		CreatorID:  user.ID,
		Content:    "test content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Get by UID
	found, err := ts.GetNote(ctx, &store.FindNote{UID: &uid})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, note.UID, found.UID)

	// Get non-existent UID
	nonExistentUID := "non-existent-uid"
	notFound, err := ts.GetNote(ctx, &store.FindNote{UID: &nonExistentUID})
	require.NoError(t, err)
	require.Nil(t, notFound)

	ts.Close()
}

func TestNoteListByVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create notes with different visibilities
	_, err = ts.CreateNote(ctx, &store.Note{
		UID:        "public-note",
		CreatorID:  user.ID,
		Content:    "public content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	_, err = ts.CreateNote(ctx, &store.Note{
		UID:        "protected-note",
		CreatorID:  user.ID,
		Content:    "protected content",
		Visibility: store.Protected,
	})
	require.NoError(t, err)

	_, err = ts.CreateNote(ctx, &store.Note{
		UID:        "private-note",
		CreatorID:  user.ID,
		Content:    "private content",
		Visibility: store.Private,
	})
	require.NoError(t, err)

	// List public notes only
	publicNotes, err := ts.ListNotes(ctx, &store.FindNote{
		VisibilityList: []store.Visibility{store.Public},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(publicNotes))
	require.Equal(t, store.Public, publicNotes[0].Visibility)

	// List protected notes only
	protectedNotes, err := ts.ListNotes(ctx, &store.FindNote{
		VisibilityList: []store.Visibility{store.Protected},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(protectedNotes))
	require.Equal(t, store.Protected, protectedNotes[0].Visibility)

	// List public and protected (multiple visibility)
	publicAndProtected, err := ts.ListNotes(ctx, &store.FindNote{
		VisibilityList: []store.Visibility{store.Public, store.Protected},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(publicAndProtected))

	// List all
	allNotes, err := ts.ListNotes(ctx, &store.FindNote{})
	require.NoError(t, err)
	require.Equal(t, 3, len(allNotes))

	ts.Close()
}

func TestNoteListWithPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create 10 notes
	for i := range 10 {
		_, err := ts.CreateNote(ctx, &store.Note{
			UID:        fmt.Sprintf("note-%d", i),
			CreatorID:  user.ID,
			Content:    fmt.Sprintf("content %d", i),
			Visibility: store.Public,
		})
		require.NoError(t, err)
	}

	// Test limit
	limit := 5
	limitedNotes, err := ts.ListNotes(ctx, &store.FindNote{Limit: &limit})
	require.NoError(t, err)
	require.Equal(t, 5, len(limitedNotes))

	// Test offset
	offset := 3
	offsetNotes, err := ts.ListNotes(ctx, &store.FindNote{Limit: &limit, Offset: &offset})
	require.NoError(t, err)
	require.Equal(t, 5, len(offsetNotes))

	// Verify offset works correctly (different notes)
	require.NotEqual(t, limitedNotes[0].ID, offsetNotes[0].ID)

	ts.Close()
}

func TestNoteUpdatePinned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "pinnable-note",
		CreatorID:  user.ID,
		Content:    "content",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	require.False(t, note.Pinned)

	// Pin the note
	pinned := true
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:     note.ID,
		Pinned: &pinned,
	})
	require.NoError(t, err)

	// Verify pinned
	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.True(t, found.Pinned)

	// Unpin
	unpinned := false
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:     note.ID,
		Pinned: &unpinned,
	})
	require.NoError(t, err)

	found, err = ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.False(t, found.Pinned)

	ts.Close()
}

func TestNoteUpdateVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "visibility-note",
		CreatorID:  user.ID,
		Content:    "content",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	require.Equal(t, store.Public, note.Visibility)

	// Change to private
	privateVisibility := store.Private
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		Visibility: &privateVisibility,
	})
	require.NoError(t, err)

	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, store.Private, found.Visibility)

	// Change to protected
	protectedVisibility := store.Protected
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		Visibility: &protectedVisibility,
	})
	require.NoError(t, err)

	found, err = ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, store.Protected, found.Visibility)

	ts.Close()
}

func TestNoteInvalidUID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create note with invalid UID (contains special characters)
	_, err = ts.CreateNote(ctx, &store.Note{
		UID:        "invalid uid with spaces",
		CreatorID:  user.ID,
		Content:    "content",
		Visibility: store.Public,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid uid")

	ts.Close()
}

func TestNoteCreateWithCustomTimestamps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	customCreatedTs := int64(1700000000) // 2023-11-14 22:13:20 UTC
	customUpdatedTs := int64(1700000001)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "custom-timestamp-note",
		CreatorID:  user.ID,
		Content:    "content with custom timestamps",
		Visibility: store.Public,
		CreatedTs:  customCreatedTs,
		UpdatedTs:  customUpdatedTs,
	})
	require.NoError(t, err)
	require.Equal(t, customCreatedTs, note.CreatedTs)
	require.Equal(t, customUpdatedTs, note.UpdatedTs)

	// Fetch and verify timestamps are preserved
	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, customCreatedTs, found.CreatedTs)
	require.Equal(t, customUpdatedTs, found.UpdatedTs)

	ts.Close()
}

func TestNoteCreateWithOnlyCreatedTs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	customCreatedTs := int64(1609459200) // 2021-01-01 00:00:00 UTC

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "custom-created-ts-only",
		CreatorID:  user.ID,
		Content:    "content with custom created_ts only",
		Visibility: store.Public,
		CreatedTs:  customCreatedTs,
	})
	require.NoError(t, err)
	require.Equal(t, customCreatedTs, note.CreatedTs)

	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, customCreatedTs, found.CreatedTs)

	ts.Close()
}

func TestNoteWithPayload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create note with tags in payload
	tags := []string{"tag1", "tag2", "tag3"}
	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-with-payload",
		CreatorID:  user.ID,
		Content:    "content with tags",
		Visibility: store.Public,
		Payload: &storepb.NotePayload{
			Tags: tags,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, note.Payload)
	require.Equal(t, tags, note.Payload.Tags)

	// Fetch and verify
	found, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.NotNil(t, found.Payload)
	require.Equal(t, tags, found.Payload.Tags)

	ts.Close()
}

package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteRelationStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	noteCreate := &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	}
	note, err := ts.CreateNote(ctx, noteCreate)
	require.NoError(t, err)
	require.Equal(t, noteCreate.Content, note.Content)
	relatedNoteCreate := &store.Note{
		UID:        "related-note",
		CreatorID:  user.ID,
		Content:    "related note content",
		Visibility: store.Public,
	}
	relatedNote, err := ts.CreateNote(ctx, relatedNoteCreate)
	require.NoError(t, err)
	require.Equal(t, relatedNoteCreate.Content, relatedNote.Content)
	commentNoteCreate := &store.Note{
		UID:        "comment-note",
		CreatorID:  user.ID,
		Content:    "comment note content",
		Visibility: store.Public,
	}
	commentNote, err := ts.CreateNote(ctx, commentNoteCreate)
	require.NoError(t, err)
	require.Equal(t, commentNoteCreate.Content, commentNote.Content)

	// Reference relation.
	referenceRelation := &store.NoteRelation{
		NoteID:        note.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	}
	_, err = ts.UpsertNoteRelation(ctx, referenceRelation)
	require.NoError(t, err)
	// Comment relation.
	commentRelation := &store.NoteRelation{
		NoteID:        note.ID,
		RelatedNoteID: commentNote.ID,
		Type:          store.NoteRelationComment,
	}
	_, err = ts.UpsertNoteRelation(ctx, commentRelation)
	require.Error(t, err)
	ts.Close()
}

func TestNoteRelationListByNoteID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create main note
	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create related notes
	relatedNote1, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-1",
		CreatorID:  user.ID,
		Content:    "related note 1 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote2, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-2",
		CreatorID:  user.ID,
		Content:    "related note 2 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relations
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote1.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote2.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// List by note ID
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(relations))

	// List by type
	refType := store.NoteRelationReference
	refRelations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
		Type:   &refType,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(refRelations))
	require.Equal(t, store.NoteRelationReference, refRelations[0].Type)

	// List by related note ID
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		RelatedNoteID: &relatedNote1.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(relations))

	ts.Close()
}

func TestNoteRelationDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create notes
	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note",
		CreatorID:  user.ID,
		Content:    "related note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relation
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Verify relation exists
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(relations))

	// Delete relation by note ID
	relType := store.NoteRelationReference
	err = ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
		NoteID:        &mainNote.ID,
		RelatedNoteID: &relatedNote.ID,
		Type:          &relType,
	})
	require.NoError(t, err)

	// Verify relation is deleted
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(relations))

	ts.Close()
}

func TestNoteRelationDeleteRequiresExplicitReferenceType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	contextNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "delete-guard-context",
		CreatorID:  user.ID,
		Content:    "context",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	comment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID:        "delete-guard-comment",
		CreatorID:  user.ID,
		Content:    "comment",
		Visibility: store.Private,
	}, contextNote.ID, user.ID)
	require.NoError(t, err)

	commentType := store.NoteRelationComment
	referenceType := store.NoteRelationReference
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        comment.ID,
		RelatedNoteID: contextNote.ID,
		Type:          referenceType,
	})
	require.NoError(t, err)

	assertRelationExists := func(t *testing.T, relationType store.NoteRelationType) {
		t.Helper()
		relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
			NoteID: &comment.ID,
			Type:   &relationType,
		})
		require.NoError(t, err)
		require.Len(t, relations, 1)
		require.Equal(t, contextNote.ID, relations[0].RelatedNoteID)
	}
	assertRelationsExist := func(t *testing.T) {
		t.Helper()
		assertRelationExists(t, commentType)
		assertRelationExists(t, referenceType)
	}
	assertRelationsExist(t)

	zeroID := int32(0)
	negativeID := int32(-1)

	for _, test := range []struct {
		name string
		run  func() error
	}{
		{
			name: "store rejects omitted type",
			run: func() error {
				return ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
					NoteID: &comment.ID, RelatedNoteID: &contextNote.ID,
				})
			},
		},
		{
			name: "driver rejects omitted type",
			run: func() error {
				return ts.GetDriver().DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
					NoteID: &comment.ID, RelatedNoteID: &contextNote.ID,
				})
			},
		},
		{
			name: "driver rejects comment type",
			run: func() error {
				return ts.GetDriver().DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
					NoteID: &comment.ID, RelatedNoteID: &contextNote.ID, Type: &commentType,
				})
			},
		},
		{
			name: "store rejects unscoped reference delete",
			run: func() error {
				return ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{Type: &referenceType})
			},
		},
		{
			name: "driver rejects unscoped reference delete",
			run: func() error {
				return ts.GetDriver().DeleteNoteRelation(ctx, &store.DeleteNoteRelation{Type: &referenceType})
			},
		},
		{
			name: "store rejects non-positive note endpoint",
			run: func() error {
				return ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{NoteID: &zeroID, Type: &referenceType})
			},
		},
		{
			name: "driver rejects non-positive related endpoint",
			run: func() error {
				return ts.GetDriver().DeleteNoteRelation(ctx, &store.DeleteNoteRelation{RelatedNoteID: &negativeID, Type: &referenceType})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.run())
			assertRelationsExist(t)
		})
	}
}

func TestNoteRelationDifferentTypes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note",
		CreatorID:  user.ID,
		Content:    "related note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create reference relation
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// COMMENT context is created atomically with its note and cannot be
	// persisted through the generic relation mutation path.
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationComment,
	})
	require.Error(t, err)

	// Verify only the reference relation exists.
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)

	ts.Close()
}

func TestNoteRelationUpsertSameRelation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note",
		CreatorID:  user.ID,
		Content:    "related note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relation
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Upsert the same relation again (should not create duplicate)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Verify only one relation exists
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)

	ts.Close()
}

func TestNoteRelationDeleteByType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote1, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-1",
		CreatorID:  user.ID,
		Content:    "related note 1 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote2, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-2",
		CreatorID:  user.ID,
		Content:    "related note 2 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create reference relations
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote1.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Create a second reference relation.
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote2.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Delete only reference type relations
	refType := store.NoteRelationReference
	err = ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
		NoteID: &mainNote.ID,
		Type:   &refType,
	})
	require.NoError(t, err)

	// Verify all reference relations were deleted.
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Empty(t, relations)

	ts.Close()
}

func TestNoteRelationDeleteByNoteID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note1, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-1",
		CreatorID:  user.ID,
		Content:    "note 1 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	note2, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-2",
		CreatorID:  user.ID,
		Content:    "note 2 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note",
		CreatorID:  user.ID,
		Content:    "related note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relations for both notes
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        note1.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        note2.ID,
		RelatedNoteID: relatedNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Delete all reference relations for note1.
	referenceType := store.NoteRelationReference
	err = ts.DeleteNoteRelation(ctx, &store.DeleteNoteRelation{
		NoteID: &note1.ID,
		Type:   &referenceType,
	})
	require.NoError(t, err)

	// Verify note1's relations are gone
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &note1.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 0)

	// Verify note2's relations still exist
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &note2.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)

	ts.Close()
}

func TestNoteRelationListByRelatedNoteID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create a note that will be referenced by others
	targetNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "target-note",
		CreatorID:  user.ID,
		Content:    "target note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create notes that reference the target
	referrer1, err := ts.CreateNote(ctx, &store.Note{
		UID:        "referrer-1",
		CreatorID:  user.ID,
		Content:    "referrer 1 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	referrer2, err := ts.CreateNote(ctx, &store.Note{
		UID:        "referrer-2",
		CreatorID:  user.ID,
		Content:    "referrer 2 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relations pointing to target
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        referrer1.ID,
		RelatedNoteID: targetNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        referrer2.ID,
		RelatedNoteID: targetNote.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// List by related note ID (find all notes that reference the target)
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		RelatedNoteID: &targetNote.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 2)

	ts.Close()
}

func TestNoteRelationListCombinedFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote1, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-1",
		CreatorID:  user.ID,
		Content:    "related note 1 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	relatedNote2, err := ts.CreateNote(ctx, &store.Note{
		UID:        "related-note-2",
		CreatorID:  user.ID,
		Content:    "related note 2 content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create multiple relations
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote1.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        mainNote.ID,
		RelatedNoteID: relatedNote2.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// List with NoteID and Type filter
	refType := store.NoteRelationReference
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID:        &mainNote.ID,
		RelatedNoteID: &relatedNote1.ID,
		Type:          &refType,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, relatedNote1.ID, relations[0].RelatedNoteID)

	// List with NoteID, RelatedNoteID, and Type filter
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID:        &mainNote.ID,
		RelatedNoteID: &relatedNote2.ID,
		Type:          &refType,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)

	ts.Close()
}

func TestNoteRelationListEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-no-relations",
		CreatorID:  user.ID,
		Content:    "note with no relations",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// List relations for note with none
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 0)

	ts.Close()
}

func TestNoteRelationBidirectional(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	noteA, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-a",
		CreatorID:  user.ID,
		Content:    "note A content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteB, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-b",
		CreatorID:  user.ID,
		Content:    "note B content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create relation A -> B
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteB.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Create relation B -> A (reverse direction)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteB.ID,
		RelatedNoteID: noteA.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Verify A -> B exists
	relationsFromA, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &noteA.ID,
	})
	require.NoError(t, err)
	require.Len(t, relationsFromA, 1)
	require.Equal(t, noteB.ID, relationsFromA[0].RelatedNoteID)

	// Verify B -> A exists
	relationsFromB, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &noteB.ID,
	})
	require.NoError(t, err)
	require.Len(t, relationsFromB, 1)
	require.Equal(t, noteA.ID, relationsFromB[0].RelatedNoteID)

	ts.Close()
}

func TestNoteRelationListByNoteIDList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create 3 notes.
	noteA, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-a",
		CreatorID:  user.ID,
		Content:    "note A content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteB, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-b",
		CreatorID:  user.ID,
		Content:    "note B content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteC, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-c",
		CreatorID:  user.ID,
		Content:    "note C content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteD, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-d",
		CreatorID:  user.ID,
		Content:    "note D content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// A -> B (reference)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteB.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// A -> C (reference)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteC.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// D -> B (reference) — B appears as related_note_id
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteD.ID,
		RelatedNoteID: noteB.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Batch query for notes A and B: should return all 3 relations
	// (A->B because A is in list, A->C because A is in list, D->B because B is in list)
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteA.ID, noteB.ID},
	})
	require.NoError(t, err)
	require.Len(t, relations, 3)

	// Batch query for note C only: should return 1 relation (A->C because C is related_note_id)
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteC.ID},
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, noteA.ID, relations[0].NoteID)
	require.Equal(t, noteC.ID, relations[0].RelatedNoteID)

	// Batch query for note D only: should return 1 relation (D->B because D is note_id)
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteD.ID},
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, noteD.ID, relations[0].NoteID)
	require.Equal(t, noteB.ID, relations[0].RelatedNoteID)

	ts.Close()
}

func TestNoteRelationListByNoteIDListEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-no-relations",
		CreatorID:  user.ID,
		Content:    "note with no relations",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Batch query with a note that has no relations.
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{note.ID},
	})
	require.NoError(t, err)
	require.Len(t, relations, 0)

	// Empty NoteIDList should not filter by NoteIDList (returns based on other filters).
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{},
	})
	require.NoError(t, err)
	require.Len(t, relations, 0)

	ts.Close()
}

func TestNoteRelationListByNoteIDListWithTypeFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	noteA, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-a",
		CreatorID:  user.ID,
		Content:    "note A content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteB, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-b",
		CreatorID:  user.ID,
		Content:    "note B content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteC, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-c",
		CreatorID:  user.ID,
		Content:    "note C content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// A -> B (reference)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteB.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// A -> C (reference)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteC.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Batch query with type filter: only references
	refType := store.NoteRelationReference
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteA.ID},
		Type:       &refType,
	})
	require.NoError(t, err)
	require.Len(t, relations, 2)
	require.Equal(t, store.NoteRelationReference, relations[0].Type)

	// Batch query with type filter: only comments
	commentType := store.NoteRelationComment
	relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteA.ID},
		Type:       &commentType,
	})
	require.NoError(t, err)
	require.Empty(t, relations)

	ts.Close()
}

func TestNoteRelationListByNoteIDListBothDirections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	noteA, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-a",
		CreatorID:  user.ID,
		Content:    "note A content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteB, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-b",
		CreatorID:  user.ID,
		Content:    "note B content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	noteX, err := ts.CreateNote(ctx, &store.Note{
		UID:        "note-x",
		CreatorID:  user.ID,
		Content:    "note X content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// X -> A (A appears as related_note_id)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteX.ID,
		RelatedNoteID: noteA.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// A -> B (A appears as note_id)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID:        noteA.ID,
		RelatedNoteID: noteB.ID,
		Type:          store.NoteRelationReference,
	})
	require.NoError(t, err)

	// Query with NoteIDList=[A]: should find both relations (A as source and A as target).
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteIDList: []int32{noteA.ID},
	})
	require.NoError(t, err)
	require.Len(t, relations, 2)

	// Verify we got both directions.
	noteIDs := map[int32]bool{}
	relatedIDs := map[int32]bool{}
	for _, r := range relations {
		noteIDs[r.NoteID] = true
		relatedIDs[r.RelatedNoteID] = true
	}
	require.True(t, noteIDs[noteX.ID], "should include X->A relation")
	require.True(t, noteIDs[noteA.ID], "should include A->B relation")
	require.True(t, relatedIDs[noteA.ID], "should include X->A relation")
	require.True(t, relatedIDs[noteB.ID], "should include A->B relation")

	ts.Close()
}

func TestNoteRelationMultipleRelationsToSameNote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	mainNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "main-note",
		CreatorID:  user.ID,
		Content:    "main note content",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	// Create multiple notes that all relate to the main note
	for i := 1; i <= 5; i++ {
		relatedNote, err := ts.CreateNote(ctx, &store.Note{
			UID:        "related-note-" + string(rune('0'+i)),
			CreatorID:  user.ID,
			Content:    "related note content",
			Visibility: store.Public,
		})
		require.NoError(t, err)

		_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
			NoteID:        mainNote.ID,
			RelatedNoteID: relatedNote.ID,
			Type:          store.NoteRelationReference,
		})
		require.NoError(t, err)
	}

	// Verify all 5 relations exist
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		NoteID: &mainNote.ID,
	})
	require.NoError(t, err)
	require.Len(t, relations, 5)

	ts.Close()
}

func TestNoteRelationFiltersSourceStatusBeforePagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	parent, err := ts.CreateNote(ctx, &store.Note{UID: "status-parent", CreatorID: user.ID, Content: "parent", Visibility: store.Public})
	require.NoError(t, err)
	normalNote, err := ts.CreateNote(ctx, &store.Note{UID: "status-normal", CreatorID: user.ID, Content: "normal", Visibility: store.Public})
	require.NoError(t, err)
	archivedNote, err := ts.CreateNote(ctx, &store.Note{UID: "status-archived", CreatorID: user.ID, Content: "archived", Visibility: store.Public})
	require.NoError(t, err)
	archived := store.Archived
	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: archivedNote.ID, RowStatus: &archived}))
	for _, noteID := range []int32{normalNote.ID, archivedNote.ID} {
		_, err := ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: noteID, RelatedNoteID: parent.ID, Type: store.NoteRelationReference})
		require.NoError(t, err)
	}

	normal := store.Normal
	referenceType := store.NoteRelationReference
	limit := 1
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{
		RelatedNoteID:       &parent.ID,
		Type:                &referenceType,
		SourceNoteRowStatus: &normal,
		Limit:               &limit,
	})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, normalNote.ID, relations[0].NoteID)
}

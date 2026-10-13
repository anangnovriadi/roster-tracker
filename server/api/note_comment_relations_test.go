package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

// TestCreateNoteComment_ReturnsParentRelation verifies that the note returned by
// CreateNoteComment carries the COMMENT relation to its parent note. The comment
// note is converted before the relation is created, so without an explicit reload
// the returned note (and the note.comment.created webhook payload built from it)
// would have an empty Relations slice. Regression test for upstream issue #6081.
func TestCreateNoteComment_ReturnsParentRelation(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	author, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "author", Role: store.RoleUser, Email: "author@example.com",
	})
	require.NoError(t, err)
	authorCtx := userCtx(ctx, author.ID)

	parent, err := svc.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "parent note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	comment, err := svc.CreateNoteComment(authorCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "a comment", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	require.Len(t, comment.Relations, 1, "comment note should carry its parent relation")
	rel := comment.Relations[0]
	assert.Equal(t, apipb.NoteRelation_COMMENT, rel.Type)
	require.NotNil(t, rel.Note)
	require.NotNil(t, rel.RelatedNote)
	assert.Equal(t, comment.Name, rel.Note.Name)
	assert.Equal(t, parent.Name, rel.RelatedNote.Name)
}

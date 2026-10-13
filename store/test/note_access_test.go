package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteAccessScopeUsesEachNoteAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "access-local-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := ts.CreateUser(ctx, &store.User{Username: "access-local-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	privateContext, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-private-context", CreatorID: owner.ID, Content: "private context", Visibility: store.Private,
	})
	require.NoError(t, err)
	publicComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "access-public-comment", CreatorID: owner.ID, Content: "public comment", Visibility: store.Public,
	}, privateContext.ID, owner.ID)
	require.NoError(t, err)
	publicContext, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-public-context", CreatorID: owner.ID, Content: "public context", Visibility: store.Public,
	})
	require.NoError(t, err)
	privateComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "access-private-comment", CreatorID: owner.ID, Content: "private comment", Visibility: store.Private,
	}, publicContext.ID, owner.ID)
	require.NoError(t, err)

	visible, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int32{publicComment.ID, publicContext.ID}, noteIDs(visible),
		"COMMENT relations must neither grant nor restrict note-local read access")

	commentsForPrivateContext, err := ts.ListNotes(ctx, &store.FindNote{
		CommentContextNoteID: &privateContext.ID,
		Access:               &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{publicComment.ID}, noteIDs(commentsForPrivateContext),
		"a readable comment may be returned even when its context note is not readable")

	nonComments, err := ts.ListNotes(ctx, &store.FindNote{
		ExcludeComments: true,
		Access:          &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{publicContext.ID}, noteIDs(nonComments), "ordinary feeds exclude notes with COMMENT context")

	ownerRows, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &owner.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int32{privateContext.ID, publicComment.ID, publicContext.ID, privateComment.ID}, noteIDs(ownerRows))
}

func TestNoteAccessScopeUsesAudienceForInvalidLocalPlacement(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "access-invalid-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: "access-invalid-note", CreatorID: owner.ID, Content: "note", Visibility: store.Public})
	require.NoError(t, err)

	_, err = ts.GetDriver().GetDB().ExecContext(ctx, fmt.Sprintf("UPDATE note SET space_id = 2147483000 WHERE id = %d", note.ID))
	require.NoError(t, err)
	visible, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &owner.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{note.ID}, noteIDs(visible), "a dangling placement must not override a non-Space audience")

	_, err = ts.GetDriver().GetDB().ExecContext(ctx, fmt.Sprintf("UPDATE note SET space_id = NULL, visibility = 'SPACE' WHERE id = %d", note.ID))
	require.NoError(t, err)
	visible, err = ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &owner.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Empty(t, visible, "SPACE without placement fails closed even for the author")
}

func TestNoteAccessScopeSpaceAudienceHasNoAuthorBypass(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	author, err := ts.CreateUser(ctx, &store.User{Username: "access-removed-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	admin, err := ts.CreateUser(ctx, &store.User{Username: "access-remaining-admin", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "access-author-space", Title: "Author access"}, author.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin}, author.ID)
	require.NoError(t, err)

	membersNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-author-members", CreatorID: author.ID, Content: "members", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	privateNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-author-private", CreatorID: author.ID, Content: "private", Visibility: store.Private, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: author.ID}, admin.ID))

	authorRows, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &author.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{privateNote.ID}, noteIDs(authorRows), "PRIVATE remains author-readable, but SPACE requires current membership")

	adminRows, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &admin.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{membersNote.ID}, noteIDs(adminRows), "membership reads SPACE but does not expand PRIVATE")
}

func TestNoteAccessScopeArchivedRowsRemainAuthorOnly(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "access-archive-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := ts.CreateUser(ctx, &store.User{Username: "access-archive-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: "access-archive-note", CreatorID: owner.ID, Content: "note", Visibility: store.Public})
	require.NoError(t, err)
	archived := store.Archived
	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, RowStatus: &archived}))

	ownerRows, err := ts.ListNotes(ctx, &store.FindNote{
		RowStatus: &archived,
		Access:    &store.NoteAccessScope{UserID: &owner.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{note.ID}, noteIDs(ownerRows))
	viewerRows, err := ts.ListNotes(ctx, &store.FindNote{
		RowStatus: &archived,
		Access:    &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.Empty(t, viewerRows)
	anonymousRows, err := ts.ListNotes(ctx, &store.FindNote{RowStatus: &archived, Access: &store.NoteAccessScope{AllowPublic: true}})
	require.NoError(t, err)
	require.Empty(t, anonymousRows)
}

func TestNoteAccessScopeAllowsArchivedCreatorButFiltersMissingCreatorBeforePagination(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	archivedCreator, err := ts.CreateUser(ctx, &store.User{Username: "access-archived-creator", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := ts.CreateUser(ctx, &store.User{Username: "access-creator-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	publicNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-archived-creator-public", CreatorID: archivedCreator.ID, Content: "public", Visibility: store.Public,
	})
	require.NoError(t, err)
	protectedNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-archived-creator-protected", CreatorID: archivedCreator.ID, Content: "protected", Visibility: store.Protected,
	})
	require.NoError(t, err)
	archived := store.Archived
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: archivedCreator.ID, RowStatus: &archived})
	require.NoError(t, err)

	visible, err := ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int32{publicNote.ID, protectedNote.ID}, noteIDs(visible),
		"archiving a creator does not change the audience of their active notes")

	missingCreatorNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "access-missing-creator", CreatorID: viewer.ID, Content: "corrupt", Visibility: store.Public,
	})
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx,
		fmt.Sprintf("UPDATE note SET creator_id = 2147483000 WHERE id = %d", missingCreatorNote.ID))
	require.NoError(t, err)

	limit := 2
	visible, err = ts.ListNotes(ctx, &store.FindNote{
		Access: &store.NoteAccessScope{UserID: &viewer.ID, AllowPublic: true, AllowProtected: true},
		Limit:  &limit,
	})
	require.NoError(t, err)
	require.Len(t, visible, 2)
	require.ElementsMatch(t, []int32{publicNote.ID, protectedNote.ID}, noteIDs(visible),
		"a newer dangling-creator row must be removed before LIMIT is applied")
}

func noteIDs(notes []*store.Note) []int32 {
	ids := make([]int32, 0, len(notes))
	for _, note := range notes {
		ids = append(ids, note.ID)
	}
	return ids
}

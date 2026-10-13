package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

// TestListsAcceptLongIDLists lists by id lists of a few hundred entries.
// Every driver must accept them: the API hands page-sized lists, up to a
// thousand entries, to these lookups.
func TestListsAcceptLongIDLists(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "long-lists", Title: "Long lists"}, user.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: "long-lists-note", CreatorID: user.ID, Content: "listed", Visibility: store.Private})
	require.NoError(t, err)
	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{UID: "long-lists-attachment", CreatorID: user.ID, Filename: "listed.png", Type: "image/png", NoteID: &note.ID})
	require.NoError(t, err)
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{CreatorID: user.ID, NoteID: note.ID, ReactionType: "👍"})
	require.NoError(t, err)

	const length = 250
	ids := make([]int32, 0, length)
	uids := make([]string, 0, length)
	usernames := make([]string, 0, length)
	for i := 1; i <= length; i++ {
		ids = append(ids, int32(i))
		uids = append(uids, fmt.Sprintf("absent-%d", i))
		usernames = append(usernames, fmt.Sprintf("absent-%d", i))
	}
	uids[0] = note.UID
	usernames[0] = user.Username

	notes, err := ts.ListNotes(ctx, &store.FindNote{UIDList: uids})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	require.Equal(t, note.ID, notes[0].ID)

	notes, err = ts.ListNotes(ctx, &store.FindNote{IDList: ids})
	require.NoError(t, err)
	require.Len(t, notes, 1)

	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{NoteIDList: ids})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, attachment.ID, attachments[0].ID)

	users, err := ts.ListUsers(ctx, &store.FindUser{IDList: ids})
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, user.ID, users[0].ID)

	users, err = ts.ListUsers(ctx, &store.FindUser{UsernameList: usernames})
	require.NoError(t, err)
	require.Len(t, users, 1)

	spaces, err := ts.ListSpaces(ctx, &store.FindSpace{IDList: ids})
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	require.Equal(t, space.ID, spaces[0].ID)

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{NoteIDList: ids})
	require.NoError(t, err)
	require.Len(t, reactions, 1)
	require.Equal(t, reaction.ID, reactions[0].ID)

	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteIDList: ids})
	require.NoError(t, err)
	require.Empty(t, relations)
}

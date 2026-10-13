package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestDeleteNoteShare_VerifiesShareBelongsToNote(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	userOne, err := ts.CreateRegularUser(ctx, "share-owner-one")
	require.NoError(t, err)
	userTwo, err := ts.CreateRegularUser(ctx, "share-owner-two")
	require.NoError(t, err)

	userOneCtx := ts.CreateUserContext(ctx, userOne.ID)
	userTwoCtx := ts.CreateUserContext(ctx, userTwo.ID)

	noteOne, err := ts.Service.CreateNote(userOneCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note one",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	noteTwo, err := ts.Service.CreateNote(userTwoCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note two",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	share, err := ts.Service.CreateNoteShare(userTwoCtx, &apipb.CreateNoteShareRequest{
		Parent:    noteTwo.Name,
		NoteShare: &apipb.NoteShare{},
	})
	require.NoError(t, err)

	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]
	forgedName := noteOne.Name + "/shares/" + shareToken

	_, err = ts.Service.DeleteNoteShare(userOneCtx, &apipb.DeleteNoteShareRequest{
		Name: forgedName,
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))

	sharedNote, err := ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: shareToken,
	})
	require.NoError(t, err)
	require.Equal(t, noteTwo.Name, sharedNote.Name)
}

func TestDeleteNoteShare_RevalidatesSpaceWriteAuthority(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "share-space-owner")
	require.NoError(t, err)
	admin, err := ts.CreateRegularUser(ctx, "share-space-admin")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "share-space", Title: "Share Space"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin,
	}, owner.ID)
	require.NoError(t, err)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "assigned share", Visibility: apipb.Visibility_PUBLIC, Space: new("spaces/" + space.UID),
	}})
	require.NoError(t, err)
	share, err := ts.Service.CreateNoteShare(ownerCtx, &apipb.CreateNoteShareRequest{Parent: note.Name, NoteShare: &apipb.NoteShare{}})
	require.NoError(t, err)
	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, admin.ID))
	_, err = ts.Service.DeleteNoteShare(ownerCtx, &apipb.DeleteNoteShareRequest{Name: share.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	requireSharePresent(ctx, t, ts, shareToken)

	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID, UserID: owner.ID, Role: store.SpaceMemberRoleUser,
	}, admin.ID)
	require.NoError(t, err)
	_, err = ts.Service.DeleteNoteShare(ownerCtx, &apipb.DeleteNoteShareRequest{Name: share.Name})
	require.NoError(t, err)
	storedShare, err := ts.Store.GetNoteShare(ctx, &store.FindNoteShare{UID: &shareToken})
	require.NoError(t, err)
	require.Nil(t, storedShare)
}

func TestDeleteNoteShare_OwnerCanRevokeLegacyAdminShare(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "legacy-share-owner")
	require.NoError(t, err)
	legacyAdmin, err := ts.CreateHostUser(ctx, "legacy-share-admin")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "note with a legacy admin share",
		Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	noteUID := strings.TrimPrefix(note.Name, "notes/")
	storedNote, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	require.NoError(t, err)
	require.NotNil(t, storedNote)

	const shareToken = "legacy-admin-created-share"
	_, err = ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       shareToken,
		NoteID:    storedNote.ID,
		CreatorID: legacyAdmin.ID,
	})
	require.NoError(t, err)

	_, err = ts.Service.DeleteNoteShare(ownerCtx, &apipb.DeleteNoteShareRequest{
		Name: note.Name + "/shares/" + shareToken,
	})
	require.NoError(t, err)

	share, err := ts.Store.GetNoteShare(ctx, &store.FindNoteShare{UID: new(shareToken)})
	require.NoError(t, err)
	require.Nil(t, share)
	_, err = ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{ShareToken: shareToken})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetSharedNote_IncludesReactions(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "share-reactions")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with reactions",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	reaction, err := ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reaction)

	share, err := ts.Service.CreateNoteShare(userCtx, &apipb.CreateNoteShareRequest{
		Parent:    note.Name,
		NoteShare: &apipb.NoteShare{},
	})
	require.NoError(t, err)

	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]
	sharedNote, err := ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: shareToken,
	})
	require.NoError(t, err)
	require.Len(t, sharedNote.Reactions, 1)
	require.Equal(t, "👍", sharedNote.Reactions[0].ReactionType)
	require.Equal(t, reaction.Name, sharedNote.Reactions[0].Name)
}

func TestCreateNoteShare_SharesOnlyTheCommentNote(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "share-single-note")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	parent, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "parent must not be shared",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	comment, err := ts.Service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
		Parent: parent.Name,
		Comment: &apipb.Note{
			Content:    "only this note is shared",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, comment.Relations)

	regularNote, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: comment.Name})
	require.NoError(t, err)
	require.NotEmpty(t, regularNote.GetParent())
	require.NotEmpty(t, regularNote.Relations)

	share, err := ts.Service.CreateNoteShare(userCtx, &apipb.CreateNoteShareRequest{
		Parent:    comment.Name,
		NoteShare: &apipb.NoteShare{},
	})
	require.NoError(t, err)
	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]
	sharedComment, err := ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{ShareToken: shareToken})
	require.NoError(t, err)
	require.Equal(t, comment.Name, sharedComment.Name)
	require.Equal(t, "only this note is shared", sharedComment.Content)
	require.Empty(t, sharedComment.GetParent())
	require.Empty(t, sharedComment.Relations)
}

func TestGetSharedNote_SkipsReactionsWithMissingCreators(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "share-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	reactor, err := ts.CreateRegularUser(ctx, "share-reaction-orphan")
	require.NoError(t, err)
	reactorCtx := ts.CreateUserContext(ctx, reactor.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with orphan share reaction",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.UpsertNoteReaction(reactorCtx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})
	require.NoError(t, err)

	share, err := ts.Service.CreateNoteShare(ownerCtx, &apipb.CreateNoteShareRequest{
		Parent:    note.Name,
		NoteShare: &apipb.NoteShare{},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: reactor.ID})
	require.NoError(t, err)

	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]
	sharedNote, err := ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: shareToken,
	})
	require.NoError(t, err)
	require.Empty(t, sharedNote.Reactions)
}

func TestGetSharedNote_ReturnsNotFoundForUnknownShare(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	_, err := ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: "missing-share-token",
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetSharedNote_ReturnsNotFoundForExpiredShare(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "share-expired")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with expired share",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	expiredTsSec := time.Now().Add(-time.Hour).Unix()
	expiredShare, err := ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "expired-share-token",
		NoteID:    parseNoteIDFromNameForTest(t, ts, note.Name),
		CreatorID: user.ID,
		ExpiresTs: &expiredTsSec,
	})
	require.NoError(t, err)

	_, err = ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: expiredShare.UID,
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetSharedNote_ReturnsNotFoundForArchivedNote(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "share-archived")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	noteResp, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note that will be archived",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	share, err := ts.Service.CreateNoteShare(userCtx, &apipb.CreateNoteShareRequest{
		Parent:    noteResp.Name,
		NoteShare: &apipb.NoteShare{},
	})
	require.NoError(t, err)

	noteID := parseNoteIDFromNameForTest(t, ts, noteResp.Name)
	note, err := ts.Store.GetNote(ctx, &store.FindNote{ID: &noteID})
	require.NoError(t, err)
	require.NotNil(t, note)

	archived := store.Archived
	err = ts.Store.UpdateNote(ctx, &store.UpdateNote{
		ID:        note.ID,
		RowStatus: &archived,
	})
	require.NoError(t, err)

	shareToken := share.Name[strings.LastIndex(share.Name, "/")+1:]
	_, err = ts.Service.GetSharedNote(ctx, &apipb.GetSharedNoteRequest{
		ShareToken: shareToken,
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func parseNoteIDFromNameForTest(t *testing.T, ts *TestService, noteName string) int32 {
	t.Helper()

	noteUID, ok := strings.CutPrefix(noteName, "notes/")
	require.True(t, ok, "note name must start with notes/: %s", noteName)

	note, err := ts.Store.GetNote(context.Background(), &store.FindNote{UID: &noteUID})
	require.NoError(t, err)
	require.NotNil(t, note)

	return note.ID
}

func requireSharePresent(ctx context.Context, t *testing.T, ts *TestService, token string) {
	t.Helper()
	share, err := ts.Store.GetNoteShare(ctx, &store.FindNoteShare{UID: &token})
	require.NoError(t, err)
	require.NotNil(t, share)
}

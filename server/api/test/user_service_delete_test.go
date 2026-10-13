package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/server/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestDeleteUserSelfDeleteCleansAccountDataAndAuthCookies(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "alice")
	require.NoError(t, err)

	_, err = ts.Store.CreateUserIdentity(ctx, &store.UserIdentity{
		UserID:    user.ID,
		Provider:  "google",
		ExternUID: "alice-google-sub",
	})
	require.NoError(t, err)

	err = ts.Store.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:   "refresh-token-id",
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		CreatedAt: timestamppb.Now(),
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(headerCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.NoError(t, err)

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)

	identities, err := ts.Store.ListUserIdentities(ctx, &store.FindUserIdentity{UserID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, identities)

	refreshSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_REFRESH_TOKENS,
	})
	require.NoError(t, err)
	require.Nil(t, refreshSetting)

	carrier := api.GetHeaderCarrier(authCtx)
	require.NotNil(t, carrier)
	require.Contains(t, strings.ToLower(carrier.Get("Set-Cookie")), "roster_refresh=")
}

func TestDeleteUserSelfDeleteRemovesOnlyOwnedNotesAndResources(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "resource-owner")
	require.NoError(t, err)
	peer, err := ts.CreateRegularUser(ctx, "resource-peer")
	require.NoError(t, err)

	userCtx := ts.CreateUserContext(ctx, user.ID)
	peerCtx := ts.CreateUserContext(ctx, peer.ID)

	ownNote, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "owner note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	foreignNote, err := ts.Service.CreateNote(peerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "peer note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	peerCommentOnOwnNote, err := ts.Service.CreateNoteComment(peerCtx, &apipb.CreateNoteCommentRequest{
		Parent: ownNote.Name,
		Comment: &apipb.Note{
			Content:    "peer comment on owner note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	peerNestedCommentOnOwnNote, err := ts.Service.CreateNoteComment(peerCtx, &apipb.CreateNoteCommentRequest{
		Parent: peerCommentOnOwnNote.Name,
		Comment: &apipb.Note{
			Content:    "peer nested comment on owner note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	userCommentOnForeignNote, err := ts.Service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
		Parent: foreignNote.Name,
		Comment: &apipb.Note{
			Content:    "owner comment on peer note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	peerReplyToUserComment, err := ts.Service.CreateNoteComment(peerCtx, &apipb.CreateNoteCommentRequest{
		Parent: userCommentOnForeignNote.Name,
		Comment: &apipb.Note{
			Content:    "peer reply to owner comment",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	ownNoteUID, err := api.ExtractNoteUIDFromName(ownNote.Name)
	require.NoError(t, err)
	ownNoteStore, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &ownNoteUID})
	require.NoError(t, err)
	require.NotNil(t, ownNoteStore)

	foreignNoteUID, err := api.ExtractNoteUIDFromName(foreignNote.Name)
	require.NoError(t, err)
	foreignNoteStore, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &foreignNoteUID})
	require.NoError(t, err)
	require.NotNil(t, foreignNoteStore)
	peerCommentOnOwnNoteID := parseNoteIDFromNameForTest(t, ts, peerCommentOnOwnNote.Name)
	peerNestedCommentOnOwnNoteID := parseNoteIDFromNameForTest(t, ts, peerNestedCommentOnOwnNote.Name)
	userCommentOnForeignNoteID := parseNoteIDFromNameForTest(t, ts, userCommentOnForeignNote.Name)
	peerReplyToUserCommentID := parseNoteIDFromNameForTest(t, ts, peerReplyToUserComment.Name)

	attachedAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-owner-note",
		CreatorID: user.ID,
		Filename:  "owner.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("note"),
		NoteID:    &ownNoteStore.ID,
	})
	require.NoError(t, err)
	thumbnailCachePath := filepath.Join(ts.Profile.Data, ".thumbnail_cache", attachedAttachment.UID+".jpeg")
	motionCachePath := filepath.Join(ts.Profile.Data, ".motion_cache", attachedAttachment.UID+".mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(thumbnailCachePath), 0o755))
	require.NoError(t, os.WriteFile(thumbnailCachePath, []byte("thumb"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(motionCachePath), 0o755))
	require.NoError(t, os.WriteFile(motionCachePath, []byte("motion"), 0o644))

	unattachedAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-owner-loose",
		CreatorID: user.ID,
		Filename:  "loose.txt",
		Type:      "text/plain",
		Size:      5,
		Blob:      []byte("loose"),
	})
	require.NoError(t, err)

	peerAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-peer-keep",
		CreatorID: peer.ID,
		Filename:  "peer.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("peer"),
		NoteID:    &foreignNoteStore.ID,
	})
	require.NoError(t, err)

	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		NoteID:       ownNoteStore.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)
	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		NoteID:       userCommentOnForeignNoteID,
		ReactionType: "🔥",
	})
	require.NoError(t, err)
	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       foreignNoteStore.ID,
		ReactionType: "👋",
	})
	require.NoError(t, err)
	peerReactionOnForeignNote, err := ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		NoteID:       foreignNoteStore.ID,
		ReactionType: "✅",
	})
	require.NoError(t, err)

	_, err = ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "share-owner-ownnote",
		NoteID:    ownNoteStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)
	_, err = ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "share-owner-foreignnote",
		NoteID:    foreignNoteStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)
	peerShare, err := ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "share-peer-foreignnote",
		NoteID:    foreignNoteStore.ID,
		CreatorID: peer.ID,
	})
	require.NoError(t, err)

	_, err = ts.Store.CreateUserIdentity(ctx, &store.UserIdentity{
		UserID:    user.ID,
		Provider:  "google",
		ExternUID: "resource-owner-google-sub",
	})
	require.NoError(t, err)
	err = ts.Store.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-owner",
		TokenHash:   "pat-owner-hash",
		Description: "owner pat",
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(headerCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.NoError(t, err)

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)

	for _, noteName := range []string{
		ownNote.Name,
		userCommentOnForeignNote.Name,
	} {
		noteUID, extractErr := api.ExtractNoteUIDFromName(noteName)
		require.NoError(t, extractErr)
		note, getErr := ts.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
		require.NoError(t, getErr)
		require.Nil(t, note, noteName)
	}
	for _, noteName := range []string{
		peerCommentOnOwnNote.Name,
		peerNestedCommentOnOwnNote.Name,
		peerReplyToUserComment.Name,
	} {
		noteUID, extractErr := api.ExtractNoteUIDFromName(noteName)
		require.NoError(t, extractErr)
		note, getErr := ts.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
		require.NoError(t, getErr)
		require.NotNil(t, note, noteName)
	}

	foreignNoteAfterDelete, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &foreignNoteUID})
	require.NoError(t, err)
	require.NotNil(t, foreignNoteAfterDelete)
	nestedRelations, err := ts.Store.ListNoteRelations(ctx, &store.FindNoteRelation{NoteID: &peerNestedCommentOnOwnNoteID})
	require.NoError(t, err)
	require.Len(t, nestedRelations, 1)
	require.Equal(t, peerCommentOnOwnNoteID, nestedRelations[0].RelatedNoteID)
	for _, noteID := range []int32{ownNoteStore.ID, userCommentOnForeignNoteID} {
		incidentRelations, listErr := ts.Store.ListNoteRelations(ctx, &store.FindNoteRelation{NoteIDList: []int32{noteID}})
		require.NoError(t, listErr)
		require.Empty(t, incidentRelations)
	}
	contextlessReplyRelations, err := ts.Store.ListNoteRelations(ctx, &store.FindNoteRelation{NoteID: &peerReplyToUserCommentID})
	require.NoError(t, err)
	require.Empty(t, contextlessReplyRelations)

	for _, attachmentID := range []int32{attachedAttachment.ID, unattachedAttachment.ID} {
		attachment, getErr := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachmentID})
		require.NoError(t, getErr)
		require.Nil(t, attachment)
	}
	peerAttachmentAfterDelete, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &peerAttachment.ID})
	require.NoError(t, err)
	require.NotNil(t, peerAttachmentAfterDelete)
	_, err = os.Stat(thumbnailCachePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(motionCachePath)
	require.ErrorIs(t, err, os.ErrNotExist)

	ownNoteReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{NoteID: &ownNoteStore.ID})
	require.NoError(t, err)
	require.Empty(t, ownNoteReactions)

	userCommentReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{NoteID: &userCommentOnForeignNoteID})
	require.NoError(t, err)
	require.Empty(t, userCommentReactions)

	foreignNoteReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{NoteID: &foreignNoteStore.ID})
	require.NoError(t, err)
	require.Len(t, foreignNoteReactions, 1)
	require.Equal(t, peerReactionOnForeignNote.ID, foreignNoteReactions[0].ID)

	ownerShares, err := ts.Store.ListNoteShares(ctx, &store.FindNoteShare{CreatorID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, ownerShares)

	peerShares, err := ts.Store.ListNoteShares(ctx, &store.FindNoteShare{CreatorID: &peer.ID})
	require.NoError(t, err)
	require.Len(t, peerShares, 1)
	require.Equal(t, peerShare.ID, peerShares[0].ID)

	sentInboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{SenderID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, sentInboxes)
	receivedInboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ReceiverID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, receivedInboxes)

	identities, err := ts.Store.ListUserIdentities(ctx, &store.FindUserIdentity{UserID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, identities)

	patSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
	})
	require.NoError(t, err)
	require.Nil(t, patSetting)
}

func TestDeleteUserSpaceMembershipGuardIgnoresForce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		force  bool
		suffix string
	}{
		{name: "without force", suffix: "normal"},
		{name: "with force", force: true, suffix: "force"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := NewTestService(t)
			defer ts.Cleanup()

			ctx := context.Background()
			user, err := ts.CreateRegularUser(ctx, "membership-delete-"+tc.suffix)
			require.NoError(t, err)
			space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "membership-delete-" + tc.suffix, Title: "Membership guard"}, user.ID)
			require.NoError(t, err)
			note, err := ts.Service.CreateNote(ts.CreateUserContext(ctx, user.ID), &apipb.CreateNoteRequest{Note: &apipb.Note{
				Content:    "must survive failed account deletion",
				Visibility: apipb.Visibility_PUBLIC,
			}})
			require.NoError(t, err)

			_, err = ts.Service.DeleteUser(ts.CreateUserContext(ctx, user.ID), &apipb.DeleteUserRequest{
				Name:  api.BuildUserName(user.Username),
				Force: tc.force,
			})
			require.Equal(t, codes.FailedPrecondition, status.Code(err))
			require.ErrorContains(t, err, "leave all spaces")

			remainingUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
			require.NoError(t, err)
			require.NotNil(t, remainingUser)
			membership, err := ts.Store.GetSpaceMember(ctx, &store.FindSpaceMember{SpaceID: &space.ID, UserID: &user.ID, ViewerUserID: &user.ID})
			require.NoError(t, err)
			require.NotNil(t, membership)
			noteUID, err := api.ExtractNoteUIDFromName(note.Name)
			require.NoError(t, err)
			remainingNote, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
			require.NoError(t, err)
			require.NotNil(t, remainingNote)
		})
	}
}

func TestDeleteUserRollbackPreservesAllResources(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "rollback-owner")
	require.NoError(t, err)
	peer, err := ts.CreateRegularUser(ctx, "rollback-peer")
	require.NoError(t, err)

	userCtx := ts.CreateUserContext(ctx, user.ID)
	ownNote, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "rollback owner note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	ownNoteUID, err := api.ExtractNoteUIDFromName(ownNote.Name)
	require.NoError(t, err)
	ownNoteStore, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &ownNoteUID})
	require.NoError(t, err)
	require.NotNil(t, ownNoteStore)

	attachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-rollback-owner",
		CreatorID: user.ID,
		Filename:  "rollback.txt",
		Type:      "text/plain",
		Size:      8,
		Blob:      []byte("rollback"),
		NoteID:    &ownNoteStore.ID,
	})
	require.NoError(t, err)

	reaction, err := ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       ownNoteStore.ID,
		ReactionType: "💥",
	})
	require.NoError(t, err)

	share, err := ts.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "share-rollback-owner",
		NoteID:    ownNoteStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)

	inbox, err := ts.Store.CreateInbox(ctx, &store.Inbox{
		SenderID:   peer.ID,
		ReceiverID: user.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_COMMENT,
			Payload: &storepb.InboxMessage_NoteComment{
				NoteComment: &storepb.InboxMessage_NoteCommentPayload{
					NoteId: ownNoteStore.ID,
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.CreateUserIdentity(ctx, &store.UserIdentity{
		UserID:    user.ID,
		Provider:  "google",
		ExternUID: "rollback-owner-google-sub",
	})
	require.NoError(t, err)
	err = ts.Store.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-rollback-owner",
		TokenHash:   "pat-rollback-owner-hash",
		Description: "rollback pat",
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	failCtx := store.WithDeleteUserFailpoint(headerCtx, store.DeleteUserFailpointBeforeCommit)
	authCtx := ts.CreateUserContext(failCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "delete user failpoint before commit")

	userAfterRollback, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.NotNil(t, userAfterRollback)

	noteAfterRollback, err := ts.Store.GetNote(ctx, &store.FindNote{UID: &ownNoteUID})
	require.NoError(t, err)
	require.NotNil(t, noteAfterRollback)

	attachmentAfterRollback, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.NotNil(t, attachmentAfterRollback)

	reactionAfterRollback, err := ts.Store.GetReaction(ctx, &store.FindReaction{ID: &reaction.ID})
	require.NoError(t, err)
	require.NotNil(t, reactionAfterRollback)

	shareAfterRollback, err := ts.Store.GetNoteShare(ctx, &store.FindNoteShare{ID: &share.ID})
	require.NoError(t, err)
	require.NotNil(t, shareAfterRollback)

	inboxesAfterRollback, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ID: &inbox.ID})
	require.NoError(t, err)
	require.Len(t, inboxesAfterRollback, 1)

	identitiesAfterRollback, err := ts.Store.ListUserIdentities(ctx, &store.FindUserIdentity{UserID: &user.ID})
	require.NoError(t, err)
	require.Len(t, identitiesAfterRollback, 1)

	patSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
	})
	require.NoError(t, err)
	require.NotNil(t, patSetting)
}

func TestDeleteUserReportsPostCommitAttachmentStorageCleanupFailure(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "cleanup-failure-owner")
	require.NoError(t, err)

	_, err = ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:         "attach-cleanup-failure",
		CreatorID:   user.ID,
		Filename:    "failure.txt",
		Type:        "text/plain",
		Size:        7,
		Blob:        []byte("failure"),
		StorageType: storepb.AttachmentStorageType_LOCAL,
		Reference:   "cleanup-failure.txt",
	})
	require.NoError(t, err)
	attachmentPath := filepath.Join(ts.Profile.Data, "cleanup-failure.txt")
	require.NoError(t, os.WriteFile(attachmentPath, []byte("failure"), 0o600))

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(store.WithDeleteAttachmentStorageFailpoint(headerCtx), user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.ErrorContains(t, err, "user was deleted but attachment storage cleanup failed")
	require.ErrorContains(t, err, store.ErrDeleteAttachmentStorageFailpoint.Error())

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)
	require.FileExists(t, attachmentPath, "storage cleanup failure happens after the database deletion commits")
}

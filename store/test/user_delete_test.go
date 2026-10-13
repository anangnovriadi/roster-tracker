package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestDeleteUserIsIdempotent(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	user, err := createTestingUserWithRole(ctx, ts, "delete-idempotent", store.RoleUser)
	require.NoError(t, err)
	first, err := ts.DeleteUser(ctx, &store.DeleteUser{ID: user.ID})
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := ts.DeleteUser(ctx, &store.DeleteUser{ID: user.ID})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Empty(t, second.UserSettingKeys)
}

func TestDeleteUserBatchBoundariesAndRollback(t *testing.T) {
	for _, count := range []int{499, 500, 501, 1001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx := context.Background()
			ts := NewTestingStore(ctx, t)
			defer ts.Close()
			owner, err := createTestingUserWithRole(ctx, ts, "batch-owner", store.RoleUser)
			require.NoError(t, err)
			peer, err := createTestingUserWithRole(ctx, ts, "batch-peer", store.RoleUser)
			require.NoError(t, err)
			keepNote, err := ts.CreateNote(ctx, &store.Note{UID: "keep-note", CreatorID: peer.ID, Content: "keep", Visibility: store.Public})
			require.NoError(t, err)
			keepNote, err = ts.GetNote(ctx, &store.FindNote{ID: &keepNote.ID})
			require.NoError(t, err)
			keepAttachment, err := ts.CreateAttachment(ctx, &store.Attachment{UID: "keep-attachment", CreatorID: peer.ID,
				NoteID: &keepNote.ID, Filename: "keep.txt", Type: "text/plain", Blob: []byte("keep")})
			require.NoError(t, err)
			keepInbox, err := ts.CreateInbox(ctx, &store.Inbox{SenderID: peer.ID, ReceiverID: peer.ID, Status: store.UNREAD,
				Message: &storepb.InboxMessage{Type: storepb.InboxMessage_NOTE_MENTION}})
			require.NoError(t, err)
			keepShare, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: "keep-share", NoteID: keepNote.ID, CreatorID: peer.ID})
			require.NoError(t, err)
			keepReaction, err := ts.UpsertReaction(ctx, &store.Reaction{CreatorID: peer.ID, NoteID: keepNote.ID, ReactionType: "keep"})
			require.NoError(t, err)
			attachmentIDs := make([]int32, 0, count)
			for i := range count {
				uid := fmt.Sprintf("batch-%d", i)
				note, err := ts.CreateNote(ctx, &store.Note{UID: uid, CreatorID: owner.ID, Content: uid, Visibility: store.Public})
				require.NoError(t, err)
				creatorID := owner.ID
				if i%2 == 0 {
					creatorID = peer.ID
				}
				attachment, err := ts.CreateAttachment(ctx, &store.Attachment{UID: uid, CreatorID: creatorID, NoteID: &note.ID,
					Filename: uid + ".txt", Type: "text/plain", Blob: []byte(uid), Reference: uid})
				require.NoError(t, err)
				attachmentIDs = append(attachmentIDs, attachment.ID)
				_, err = ts.UpsertReaction(ctx, &store.Reaction{CreatorID: peer.ID, NoteID: note.ID, ReactionType: "thumbs-up"})
				require.NoError(t, err)
				_, err = ts.CreateNoteShare(ctx, &store.NoteShare{UID: uid, NoteID: note.ID, CreatorID: peer.ID})
				require.NoError(t, err)
				_, err = ts.CreateInbox(ctx, &store.Inbox{SenderID: owner.ID, ReceiverID: peer.ID, Status: store.UNREAD,
					Message: &storepb.InboxMessage{Type: storepb.InboxMessage_NOTE_MENTION}})
				require.NoError(t, err)
				// Exercise both incoming and outgoing relation cleanup across every batch.
				_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: note.ID, RelatedNoteID: keepNote.ID, Type: store.NoteRelationReference})
				require.NoError(t, err)
				_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: keepNote.ID, RelatedNoteID: note.ID, Type: store.NoteRelationReference})
				require.NoError(t, err)
			}
			_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{UserId: owner.ID, Key: storepb.UserSetting_GENERAL,
				Value: &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "en"}}})
			require.NoError(t, err)
			_, err = ts.CreateUserIdentity(ctx, &store.UserIdentity{UserID: owner.ID, Provider: "test", ExternUID: "batch-owner"})
			require.NoError(t, err)
			// Prime facade caches before forcing a failure after every delete statement.
			_, err = ts.GetUser(ctx, &store.FindUser{ID: &owner.ID})
			require.NoError(t, err)
			_, err = ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &owner.ID, Key: storepb.UserSetting_GENERAL})
			require.NoError(t, err)
			if count > 500 {
				before := snapshotUserDeletion(t, ts)
				result, err := ts.DeleteUser(store.WithDeleteUserFailpoint(ctx, store.DeleteUserFailpointBeforeCommit), &store.DeleteUser{ID: owner.ID})
				require.ErrorContains(t, err, "delete user failpoint before commit")
				require.Nil(t, result, "failed deletion must not expose resources for physical cleanup")
				require.Equal(t, before, snapshotUserDeletion(t, ts), "all batches and related tables must roll back")
				cachedUser, err := ts.GetUser(ctx, &store.FindUser{ID: &owner.ID})
				require.NoError(t, err)
				require.NotNil(t, cachedUser)
				cachedSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &owner.ID, Key: storepb.UserSetting_GENERAL})
				require.NoError(t, err)
				require.Equal(t, "en", cachedSetting.GetGeneral().GetLocale())
			}
			result, err := ts.DeleteUser(ctx, &store.DeleteUser{ID: owner.ID})
			require.NoError(t, err)
			require.ElementsMatch(t, []storepb.UserSetting_Key{storepb.UserSetting_GENERAL}, result.UserSettingKeys)
			var deletedIDs []int32
			for _, attachment := range result.Attachments {
				deletedIDs = append(deletedIDs, attachment.ID)
				require.Equal(t, attachment.UID, attachment.Reference)
			}
			require.ElementsMatch(t, attachmentIDs, deletedIDs, "each attachment must be returned exactly once, even if both owner and note match")
			after := snapshotUserDeletion(t, ts)
			require.Len(t, after.users, 1)
			require.Equal(t, peer.ID, after.users[0].ID)
			require.Equal(t, []*store.Note{keepNote}, after.notes)
			require.Len(t, after.attachments, 1)
			require.Equal(t, keepAttachment.ID, after.attachments[0].ID)
			require.Equal(t, []byte("keep"), after.attachments[0].Blob)
			require.Equal(t, []*store.NoteShare{keepShare}, after.shares)
			require.Equal(t, []*store.Reaction{keepReaction}, after.reactions)
			require.Len(t, after.inboxes, 1)
			require.Equal(t, keepInbox.ID, after.inboxes[0].ID)
			require.Empty(t, after.relations)
			require.Empty(t, after.settings)
			require.Empty(t, after.identities)
			cachedUser, err := ts.GetUser(ctx, &store.FindUser{ID: &owner.ID})
			require.NoError(t, err)
			require.Nil(t, cachedUser)
			cachedSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &owner.ID, Key: storepb.UserSetting_GENERAL})
			require.NoError(t, err)
			require.Nil(t, cachedSetting)
		})
	}
}

type userDeletionSnapshot struct {
	users       []*store.User
	notes       []*store.Note
	attachments []*store.Attachment
	shares      []*store.NoteShare
	reactions   []*store.Reaction
	inboxes     []*store.Inbox
	relations   []*store.NoteRelation
	settings    []*storepb.UserSetting
	identities  []*store.UserIdentity
}

func snapshotUserDeletion(t *testing.T, ts *store.Store) userDeletionSnapshot {
	t.Helper()
	ctx := context.Background()
	var result userDeletionSnapshot
	var err error
	result.users, err = ts.ListUsers(ctx, &store.FindUser{})
	require.NoError(t, err)
	result.notes, err = ts.ListNotes(ctx, &store.FindNote{})
	require.NoError(t, err)
	result.attachments, err = ts.ListAttachments(ctx, &store.FindAttachment{GetBlob: true, SkipDefaultLimit: true})
	require.NoError(t, err)
	result.shares, err = ts.ListNoteShares(ctx, &store.FindNoteShare{})
	require.NoError(t, err)
	result.reactions, err = ts.ListReactions(ctx, &store.FindReaction{})
	require.NoError(t, err)
	result.inboxes, err = ts.ListInboxes(ctx, &store.FindInbox{})
	require.NoError(t, err)
	result.relations, err = ts.ListNoteRelations(ctx, &store.FindNoteRelation{})
	require.NoError(t, err)
	result.settings, err = ts.ListUserSettings(ctx, &store.FindUserSetting{})
	require.NoError(t, err)
	result.identities, err = ts.ListUserIdentities(ctx, &store.FindUserIdentity{})
	require.NoError(t, err)
	return result
}

func TestDeleteUserCleansRelatedData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	user, err := createTestingUserWithRole(ctx, ts, "test", store.RoleUser)
	require.NoError(t, err)
	peer, err := createTestingUserWithRole(ctx, ts, "delete-peer", store.RoleUser)
	require.NoError(t, err)

	ownNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "delete-own-note",
		CreatorID:  user.ID,
		Content:    "owner note",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	peerNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "delete-peer-note",
		CreatorID:  peer.ID,
		Content:    "peer note",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	peerCommentOnOwnNote, err := ts.CreateNoteComment(ctx, &store.Note{
		UID:       "delete-peer-comment",
		CreatorID: peer.ID,
		Content:   "peer comment on owner note",
	}, ownNote.ID, peer.ID)
	require.NoError(t, err)
	userCommentOnPeerNote, err := ts.CreateNoteComment(ctx, &store.Note{
		UID:       "delete-user-comment",
		CreatorID: user.ID,
		Content:   "owner comment on peer note",
	}, peerNote.ID, user.ID)
	require.NoError(t, err)

	ownerAttachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "delete-owner-attachment",
		CreatorID: user.ID,
		Filename:  "owner.txt",
		Type:      "text/plain",
		Size:      5,
		Blob:      []byte("owner"),
		NoteID:    &ownNote.ID,
	})
	require.NoError(t, err)
	peerAttachmentOnDeletedNote, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "delete-peer-attachment",
		CreatorID: peer.ID,
		Filename:  "peer-on-owner.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("peer"),
		NoteID:    &ownNote.ID,
	})
	require.NoError(t, err)
	peerAttachmentToKeep, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "keep-peer-attachment",
		CreatorID: peer.ID,
		Filename:  "peer.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("peer"),
		NoteID:    &peerNote.ID,
	})
	require.NoError(t, err)

	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		NoteID:       ownNote.ID,
		ReactionType: "thumbs-up",
	})
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       peerNote.ID,
		ReactionType: "heart",
	})
	require.NoError(t, err)
	peerReactionToKeep, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		NoteID:       peerNote.ID,
		ReactionType: "sparkle",
	})
	require.NoError(t, err)

	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "delete-owner-share",
		NoteID:    peerNote.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "delete-note-share",
		NoteID:    ownNote.ID,
		CreatorID: peer.ID,
	})
	require.NoError(t, err)
	peerShareToKeep, err := ts.CreateNoteShare(ctx, &store.NoteShare{
		UID:       "keep-peer-share",
		NoteID:    peerNote.ID,
		CreatorID: peer.ID,
	})
	require.NoError(t, err)

	_, err = ts.CreateInbox(ctx, &store.Inbox{
		SenderID:   user.ID,
		ReceiverID: peer.ID,
		Status:     store.UNREAD,
		Message:    &storepb.InboxMessage{Type: storepb.InboxMessage_NOTE_MENTION},
	})
	require.NoError(t, err)
	referencingDeletedNoteInbox, err := ts.CreateInbox(ctx, &store.Inbox{
		SenderID:   peer.ID,
		ReceiverID: peer.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_COMMENT,
			Payload: &storepb.InboxMessage_NoteComment{
				NoteComment: &storepb.InboxMessage_NoteCommentPayload{
					NoteId: ownNote.ID,
				},
			},
		},
	})
	require.NoError(t, err)
	inboxToKeep, err := ts.CreateInbox(ctx, &store.Inbox{
		SenderID:   peer.ID,
		ReceiverID: peer.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_COMMENT,
			Payload: &storepb.InboxMessage_NoteComment{
				NoteComment: &storepb.InboxMessage_NoteCommentPayload{
					NoteId: peerNote.ID,
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = ts.CreateUserIdentity(ctx, &store.UserIdentity{
		UserID:    user.ID,
		Provider:  "google",
		ExternUID: "delete-user-sub",
	})
	require.NoError(t, err)
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "delete-user-pat",
		TokenHash:   "delete-user-pat-hash",
		Description: "delete user pat",
	})
	require.NoError(t, err)

	_, err = ts.DeleteUser(ctx, &store.DeleteUser{ID: user.ID})
	require.NoError(t, err)

	deletedUser, err := ts.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)
	keptUser, err := ts.GetUser(ctx, &store.FindUser{ID: &peer.ID})
	require.NoError(t, err)
	require.NotNil(t, keptUser)

	for _, note := range []*store.Note{ownNote, userCommentOnPeerNote} {
		got, getErr := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
		require.NoError(t, getErr)
		require.Nil(t, got, note.UID)
	}
	for _, note := range []*store.Note{peerNote, peerCommentOnOwnNote} {
		got, getErr := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
		require.NoError(t, getErr)
		require.NotNil(t, got, note.UID)
	}

	for _, attachment := range []*store.Attachment{ownerAttachment, peerAttachmentOnDeletedNote} {
		got, getErr := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
		require.NoError(t, getErr)
		require.Nil(t, got, attachment.UID)
	}
	keptAttachment, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &peerAttachmentToKeep.ID})
	require.NoError(t, err)
	require.NotNil(t, keptAttachment)

	deletedNoteRelations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteIDList: []int32{ownNote.ID, peerCommentOnOwnNote.ID, userCommentOnPeerNote.ID}})
	require.NoError(t, err)
	require.Empty(t, deletedNoteRelations, "relations incident to deleted notes must be removed without deleting the other endpoint")

	keptReactions, err := ts.ListReactions(ctx, &store.FindReaction{NoteID: &peerNote.ID})
	require.NoError(t, err)
	require.Len(t, keptReactions, 1)
	require.Equal(t, peerReactionToKeep.ID, keptReactions[0].ID)

	deletedOwnerShares, err := ts.ListNoteShares(ctx, &store.FindNoteShare{CreatorID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, deletedOwnerShares)
	keptShare, err := ts.GetNoteShare(ctx, &store.FindNoteShare{ID: &peerShareToKeep.ID})
	require.NoError(t, err)
	require.NotNil(t, keptShare)

	deletedSentInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{SenderID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, deletedSentInboxes)
	deletedReceivedInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ReceiverID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, deletedReceivedInboxes)
	referencedInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ID: &referencingDeletedNoteInbox.ID})
	require.NoError(t, err)
	require.Len(t, referencedInboxes, 1, "deleting a referenced note must not control another user's inbox lifecycle")
	keptInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ID: &inboxToKeep.ID})
	require.NoError(t, err)
	require.Len(t, keptInboxes, 1)

	identities, err := ts.ListUserIdentities(ctx, &store.FindUserIdentity{UserID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, identities)
	setting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
	})
	require.NoError(t, err)
	require.Nil(t, setting)
}

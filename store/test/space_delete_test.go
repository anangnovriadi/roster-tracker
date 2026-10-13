package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestDeleteSpaceDeletesOnlyDirectlyAssignedNotesAndOwnedResources(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	admin, err := ts.CreateUser(ctx, &store.User{Username: "space-delete-admin", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "space-delete-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	applicationAdmin, err := ts.CreateUser(ctx, &store.User{Username: "space-delete-app-admin", Role: store.RoleAdmin, PasswordHash: "hash"})
	require.NoError(t, err)

	space, err := ts.CreateSpace(ctx, &store.Space{UID: "delete-space", Title: "Delete Space"}, admin.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, admin.ID)
	require.NoError(t, err)

	assigned, err := ts.CreateNote(ctx, &store.Note{
		UID:        "delete-space-assigned",
		CreatorID:  admin.ID,
		Content:    "assigned content",
		Visibility: store.Public,
		SpaceID:    &space.ID,
	})
	require.NoError(t, err)
	survivor, err := ts.CreateNote(ctx, &store.Note{UID: "delete-space-survivor", CreatorID: member.ID, Content: "survivor", Visibility: store.Public})
	require.NoError(t, err)
	otherSurvivor, err := ts.CreateNote(ctx, &store.Note{UID: "delete-space-other-survivor", CreatorID: member.ID, Content: "other", Visibility: store.Public})
	require.NoError(t, err)

	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: assigned.ID, RelatedNoteID: survivor.ID, Type: store.NoteRelationReference})
	require.NoError(t, err)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: otherSurvivor.ID, RelatedNoteID: assigned.ID, Type: store.NoteRelationReference})
	require.NoError(t, err)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{NoteID: survivor.ID, RelatedNoteID: otherSurvivor.ID, Type: store.NoteRelationReference})
	require.NoError(t, err)

	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "delete-space-attachment",
		CreatorID: admin.ID,
		Filename:  "assigned.txt",
		NoteID:    &assigned.ID,
		Payload:   &storepb.AttachmentPayload{},
	})
	require.NoError(t, err)
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{CreatorID: member.ID, NoteID: assigned.ID, ReactionType: "THUMBS_UP"})
	require.NoError(t, err)
	share, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: "delete-space-share", NoteID: assigned.ID, CreatorID: admin.ID})
	require.NoError(t, err)
	referencingNotification, err := ts.CreateInbox(ctx, &store.Inbox{
		SenderID: admin.ID, ReceiverID: member.ID, Status: store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_COMMENT,
			Payload: &storepb.InboxMessage_NoteComment{NoteComment: &storepb.InboxMessage_NoteCommentPayload{
				NoteId: assigned.ID, RelatedNoteId: survivor.ID,
			}},
		},
	})
	require.NoError(t, err)
	survivingNotification, err := ts.CreateInbox(ctx, &store.Inbox{
		SenderID: member.ID, ReceiverID: admin.ID, Status: store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_NOTE_MENTION,
			Payload: &storepb.InboxMessage_NoteMention{NoteMention: &storepb.InboxMessage_NoteMentionPayload{
				NoteId: survivor.ID, RelatedNoteId: otherSurvivor.ID,
			}},
		},
	})
	require.NoError(t, err)

	_, err = ts.DeleteSpace(ctx, &store.DeleteSpace{ID: space.ID, ActorUserID: member.ID})
	require.ErrorIs(t, err, store.ErrSpacePermissionDenied)
	_, err = ts.DeleteSpace(ctx, &store.DeleteSpace{ID: space.ID, ActorUserID: applicationAdmin.ID})
	require.ErrorIs(t, err, store.ErrSpacePermissionDenied, "application ADMIN must not bypass Space membership")

	deleteResult, err := ts.DeleteSpace(ctx, &store.DeleteSpace{ID: space.ID, ActorUserID: admin.ID})
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	require.Len(t, deleteResult.Attachments, 1)
	require.Equal(t, attachment.ID, deleteResult.Attachments[0].ID)

	deletedSpace, err := ts.GetSpace(ctx, &store.FindSpace{ID: &space.ID})
	require.NoError(t, err)
	require.Nil(t, deletedSpace)
	deletedNote, err := ts.GetNote(ctx, &store.FindNote{ID: &assigned.ID})
	require.NoError(t, err)
	require.Nil(t, deletedNote)
	for _, noteID := range []int32{survivor.ID, otherSurvivor.ID} {
		note, err := ts.GetNote(ctx, &store.FindNote{ID: &noteID})
		require.NoError(t, err)
		require.NotNil(t, note, "relation traversal must not delete surviving endpoints")
	}

	deletedAttachment, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Nil(t, deletedAttachment)
	deletedReaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reaction.ID})
	require.NoError(t, err)
	require.Nil(t, deletedReaction)
	deletedShare, err := ts.GetNoteShare(ctx, &store.FindNoteShare{ID: &share.ID})
	require.NoError(t, err)
	require.Nil(t, deletedShare)
	retainedInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ID: &referencingNotification.ID})
	require.NoError(t, err)
	require.Len(t, retainedInboxes, 1, "Space deletion must not control the independent inbox lifecycle")
	survivingInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ID: &survivingNotification.ID})
	require.NoError(t, err)
	require.Len(t, survivingInboxes, 1)

	incidentRelations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteID: &assigned.ID})
	require.NoError(t, err)
	require.Empty(t, incidentRelations)
	incomingIncidentRelations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{RelatedNoteID: &assigned.ID})
	require.NoError(t, err)
	require.Empty(t, incomingIncidentRelations)
	survivingRelations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteID: &survivor.ID})
	require.NoError(t, err)
	require.Len(t, survivingRelations, 1)
	require.Equal(t, otherSurvivor.ID, survivingRelations[0].RelatedNoteID)

	membership, err := ts.GetSpaceMember(ctx, &store.FindSpaceMember{SpaceID: &space.ID, UserID: &member.ID})
	require.NoError(t, err)
	require.Nil(t, membership)
}

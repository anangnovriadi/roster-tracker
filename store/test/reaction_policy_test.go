package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestReactionWritePolicyUsesTargetNoteAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-local-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := ts.CreateUser(ctx, &store.User{Username: "reaction-local-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	outsider, err := ts.CreateUser(ctx, &store.User{Username: "reaction-local-outsider", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	contextNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "reaction-local-context", CreatorID: owner.ID, Content: "public context", Visibility: store.Public,
	})
	require.NoError(t, err)
	privateComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "reaction-local-private-comment", CreatorID: viewer.ID, Content: "private comment", Visibility: store.Private,
	}, contextNote.ID, viewer.ID)
	require.NoError(t, err)

	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: owner.ID, NoteID: privateComment.ID, ReactionType: "context-author",
		Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrNotePermissionDenied, "a public context relation must not grant access to a private comment")
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: viewer.ID, NoteID: privateComment.ID, ReactionType: "comment-author",
		Policy: reactionWritePolicy(viewer.ID),
	})
	require.NoError(t, err)

	privateContext, err := ts.CreateNote(ctx, &store.Note{
		UID: "reaction-local-private-context", CreatorID: owner.ID, Content: "private context", Visibility: store.Private,
	})
	require.NoError(t, err)
	publicComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "reaction-local-public-comment", CreatorID: owner.ID, Content: "public comment", Visibility: store.Public,
	}, privateContext.ID, owner.ID)
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: outsider.ID, NoteID: publicComment.ID, ReactionType: "public-comment",
		Policy: reactionWritePolicy(outsider.ID),
	})
	require.NoError(t, err, "a private context relation must not restrict a public comment")

	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: outsider.ID, NoteID: publicComment.ID, ReactionType: "wrong-actor", Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrReactionPermissionDenied)
}

func TestReactionWritePolicySpaceParticipationIsNoteLocal(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-space-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "reaction-space-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	outsider, err := ts.CreateUser(ctx, &store.User{Username: "reaction-space-outsider", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	applicationAdmin, err := ts.CreateUser(ctx, &store.User{Username: "reaction-space-app-admin", Role: store.RoleAdmin, PasswordHash: "hash"})
	require.NoError(t, err)

	space, err := ts.CreateSpace(ctx, &store.Space{UID: "reaction-space", Title: "Reaction Space"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)
	spaceNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "reaction-space-note", CreatorID: owner.ID, Content: "space note", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	})
	require.NoError(t, err)

	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: member.ID, NoteID: spaceNote.ID, ReactionType: "member", Policy: reactionWritePolicy(member.ID),
	})
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: outsider.ID, NoteID: spaceNote.ID, ReactionType: "outsider", Policy: reactionWritePolicy(outsider.ID),
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired, "a non-member must not participate")
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: applicationAdmin.ID, NoteID: spaceNote.ID, ReactionType: "application-admin", Policy: reactionWritePolicy(applicationAdmin.ID),
	})
	require.NoError(t, err, "an instance administrator participates without membership")

	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: member.ID}, owner.ID))
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: member.ID, NoteID: spaceNote.ID, ReactionType: "former-member", Policy: reactionWritePolicy(member.ID),
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)

	assignedPrivate, err := ts.CreateNote(ctx, &store.Note{
		UID: "reaction-space-private", CreatorID: owner.ID, Content: "private", Visibility: store.Private, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: member.ID, NoteID: assignedPrivate.ID, ReactionType: "private-member", Policy: reactionWritePolicy(member.ID),
	})
	require.ErrorIs(t, err, store.ErrNotePermissionDenied, "Space membership must not broaden a PRIVATE note")
}

func TestReactionWritePolicyRejectsInactiveActorAndNote(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-state-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	actor, err := ts.CreateUser(ctx, &store.User{Username: "reaction-state-actor", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: "reaction-state-note", CreatorID: owner.ID, Content: "note", Visibility: store.Public})
	require.NoError(t, err)

	archived := store.Archived
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: actor.ID, RowStatus: &archived})
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: actor.ID, NoteID: note.ID, ReactionType: "inactive-actor", Policy: reactionWritePolicy(actor.ID),
	})
	require.ErrorIs(t, err, store.ErrReactionPermissionDenied)

	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, RowStatus: &archived}))
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: owner.ID, NoteID: note.ID, ReactionType: "inactive-note", Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceNotWritable)
}

func TestDeleteReactionAtomicallyEnforcesCreatorWithoutParticipation(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-delete-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "reaction-delete-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "reaction-delete-space", Title: "Delete Reactions"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "reaction-delete-note", CreatorID: owner.ID, Content: "note", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: member.ID, NoteID: note.ID, ReactionType: "delete", Policy: reactionWritePolicy(member.ID),
	})
	require.NoError(t, err)

	err = ts.DeleteReaction(ctx, &store.DeleteReaction{
		ID: &reaction.ID, NoteID: &note.ID, ActorUserID: &owner.ID, Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrReactionPermissionDenied)
	requireReactionPresent(ctx, t, ts, reaction.ID)

	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: member.ID}, owner.ID))
	err = ts.DeleteReaction(ctx, &store.DeleteReaction{
		ID: &reaction.ID, NoteID: &note.ID, ActorUserID: &member.ID, Policy: reactionWritePolicy(member.ID),
	})
	require.NoError(t, err, "creators can withdraw reactions after leaving the Space")

	requireReactionMissing(ctx, t, ts, reaction.ID)
}

func reactionWritePolicy(actorUserID int32) *store.ReactionWritePolicy {
	return &store.ReactionWritePolicy{ActorUserID: actorUserID}
}

func requireReactionPresent(ctx context.Context, t *testing.T, ts *store.Store, reactionID int32) {
	t.Helper()
	reaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reactionID})
	require.NoError(t, err)
	require.NotNil(t, reaction)
}

func requireReactionMissing(ctx context.Context, t *testing.T, ts *store.Store, reactionID int32) {
	t.Helper()
	reaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reactionID})
	require.NoError(t, err)
	require.Nil(t, reaction)
}

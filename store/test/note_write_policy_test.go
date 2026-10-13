package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteWritePolicyRevalidatesMembershipAndExactNoteShares(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "note-policy-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	admin, err := ts.CreateUser(ctx, &store.User{Username: "note-policy-admin", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "note-policy-space", Title: "Policy"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: "note-policy-root", CreatorID: owner.ID, Content: "note", Visibility: store.Public, SpaceID: &space.ID})
	require.NoError(t, err)
	comment, err := ts.CreateNoteComment(ctx, &store.Note{UID: "note-policy-comment", CreatorID: owner.ID, Content: "comment"}, note.ID, owner.ID)
	require.NoError(t, err)

	// A share on a related note does not affect this note's audience.
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{UID: "note-policy-comment-share", NoteID: comment.ID, CreatorID: owner.ID})
	require.NoError(t, err)
	spaceAudience := store.SpaceAudience
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		Visibility: &spaceAudience,
		Policy:     noteWritePolicy(owner.ID, false),
	})
	require.NoError(t, err)

	public := store.Public
	err = ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, Visibility: &public})
	require.NoError(t, err)
	note.Visibility = store.Public
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{UID: "note-policy-direct-share", NoteID: note.ID, CreatorID: owner.ID})
	require.NoError(t, err)
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		Visibility: &spaceAudience,
		Policy:     noteWritePolicy(owner.ID, false),
	})
	require.ErrorIs(t, err, store.ErrNoteShareConflict)

	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, admin.ID))
	updatedContent := "must not commit"
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:      note.ID,
		Content: &updatedContent,
		Policy:  noteWritePolicy(owner.ID, false),
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)

	// The explicit lifecycle exception still permits withdrawal after removal.
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		ClearSpace: true,
		Policy:     noteWritePolicy(owner.ID, true),
	})
	require.NoError(t, err)
	note, err = ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Nil(t, note.SpaceID)
	require.Equal(t, "note", note.Content)
}

func TestNoteWritePolicyIgnoresExpiredSharesWhenNarrowingAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "note-expired-share-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "note-expired-share-space", Title: "Expired share"}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "note-expired-share-root", CreatorID: owner.ID, Content: "note", Visibility: store.Public, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	expiredTsSec := time.Now().Add(-time.Hour).Unix()
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{
		UID: "note-expired-share-token", NoteID: note.ID, CreatorID: owner.ID, ExpiresTs: &expiredTsSec,
	})
	require.NoError(t, err)

	spaceAudience := store.SpaceAudience
	err = ts.UpdateNote(ctx, &store.UpdateNote{
		ID:         note.ID,
		Visibility: &spaceAudience,
		Policy:     noteWritePolicy(owner.ID, false),
	})
	require.NoError(t, err)
}

func TestNoteWritePolicyRemovedMemberCanOnlyMoveOrWithdraw(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "note-move-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	admin, err := ts.CreateUser(ctx, &store.User{Username: "note-move-admin", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	source, err := ts.CreateSpace(ctx, &store.Space{UID: "note-move-source", Title: "Source"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: source.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin}, owner.ID)
	require.NoError(t, err)
	target, err := ts.CreateSpace(ctx, &store.Space{UID: "note-move-target", Title: "Target"}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "note-move", CreatorID: owner.ID, Content: "note", Visibility: store.Public, SpaceID: &source.ID,
	})
	require.NoError(t, err)
	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: source.ID, UserID: owner.ID}, admin.ID))

	policy := noteWritePolicy(owner.ID, true)
	newContent := "must not commit"
	err = ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, SpaceID: &target.ID, Content: &newContent, Policy: policy})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)

	err = ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, SpaceID: &target.ID, Policy: policy})
	require.NoError(t, err)
	note, err = ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, &target.ID, note.SpaceID)
	require.Equal(t, "note", note.Content)
}

func TestNoteWritePolicyUsesCurrentAudienceForMoveRules(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "note-current-audience-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	source, err := ts.CreateSpace(ctx, &store.Space{UID: "note-current-audience-source", Title: "Source"}, owner.ID)
	require.NoError(t, err)
	target, err := ts.CreateSpace(ctx, &store.Space{UID: "note-current-audience-target", Title: "Target"}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "note-current-audience", CreatorID: owner.ID, Content: "note", Visibility: store.Public, SpaceID: &source.ID,
	})
	require.NoError(t, err)

	// Simulate the audience changing after the transport read but before its
	// policy-backed move. The transaction must use SPACE as the current
	// audience and require explicit confirmation.
	spaceAudience := store.SpaceAudience
	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, Visibility: &spaceAudience}))
	policy := noteWritePolicy(owner.ID, true)
	err = ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, SpaceID: &target.ID, Policy: policy})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)

	err = ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, SpaceID: &target.ID, Visibility: &spaceAudience, Policy: policy})
	require.NoError(t, err)
	stored, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, &target.ID, stored.SpaceID)
	require.Equal(t, store.SpaceAudience, stored.Visibility)
}

func TestNoteWritePolicyUsesCurrentAudienceWhenCreatingShare(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "note-current-share-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "note-current-share-space", Title: "Share"}, owner.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "note-current-share", CreatorID: owner.ID, Content: "note", Visibility: store.Public, SpaceID: &space.ID,
	})
	require.NoError(t, err)

	policy := &store.NoteWritePolicy{ActorUserID: owner.ID, CreatingShare: true}
	spaceAudience := store.SpaceAudience
	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, Visibility: &spaceAudience}))
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{
		UID: "note-current-share-token", NoteID: note.ID, CreatorID: owner.ID, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrNoteShareConflict)

	public := store.Public
	archived := store.Archived
	require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, Visibility: &public, RowStatus: &archived}))
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{
		UID: "note-current-archived-share", NoteID: note.ID, CreatorID: owner.ID, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)
}

func noteWritePolicy(actorUserID int32, lifecycleOnly bool) *store.NoteWritePolicy {
	return &store.NoteWritePolicy{ActorUserID: actorUserID, LifecycleOnly: lifecycleOnly}
}

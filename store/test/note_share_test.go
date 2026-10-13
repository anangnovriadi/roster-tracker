package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteShareDeleteRequiresSelector(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "note-share-delete-selector", CreatorID: owner.ID, Content: "note", Visibility: store.Public,
	})
	require.NoError(t, err)

	for _, uid := range []string{"note-share-delete-first", "note-share-delete-second"} {
		_, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: uid, NoteID: note.ID, CreatorID: owner.ID})
		require.NoError(t, err)
	}

	require.Error(t, ts.DeleteNoteShare(ctx, &store.DeleteNoteShare{}))
	require.Error(t, ts.DeleteNoteShare(ctx, &store.DeleteNoteShare{NoteID: &note.ID}))
	shares, err := ts.ListNoteShares(ctx, &store.FindNoteShare{NoteID: &note.ID})
	require.NoError(t, err)
	require.Len(t, shares, 2)

	require.NoError(t, ts.DeleteNoteShare(ctx, &store.DeleteNoteShare{UID: &shares[0].UID}))
	shares, err = ts.ListNoteShares(ctx, &store.FindNoteShare{NoteID: &note.ID})
	require.NoError(t, err)
	require.Len(t, shares, 1)
}

func TestNoteShareAuthorizedDeletion(t *testing.T) {
	for _, name := range []string{
		"owner by id", "owner by uid", "matching selectors", "instance admin", "archived note",
		"unrelated actor", "archived actor", "missing actor", "demoted admin", "removed membership",
		"mismatched note", "mismatched selectors", "missing share", "missing note", "database failure",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ts := NewTestingStore(ctx, t)
			defer ts.Close()
			owner, err := createTestingUserWithRole(ctx, ts, "share-owner", store.RoleUser)
			require.NoError(t, err)
			admin, err := createTestingUserWithRole(ctx, ts, "share-admin", store.RoleAdmin)
			require.NoError(t, err)
			peer, err := createTestingUserWithRole(ctx, ts, "share-peer", store.RoleUser)
			require.NoError(t, err)
			space, err := ts.CreateSpace(ctx, &store.Space{UID: "share-space", Title: "Shares"}, owner.ID)
			require.NoError(t, err)
			_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: peer.ID, Role: store.SpaceMemberRoleAdmin}, owner.ID)
			require.NoError(t, err)
			note, err := ts.CreateNote(ctx, &store.Note{UID: "share-note", CreatorID: owner.ID,
				Content: "note", Visibility: store.Public, SpaceID: &space.ID})
			require.NoError(t, err)
			other, err := ts.CreateNote(ctx, &store.Note{UID: "other-share-note", CreatorID: owner.ID,
				Content: "other", Visibility: store.Public})
			require.NoError(t, err)
			share, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: "share-token", NoteID: note.ID,
				CreatorID: owner.ID, Policy: &store.NoteWritePolicy{ActorUserID: owner.ID, CreatingShare: true}})
			require.NoError(t, err)
			keep, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: "keep-token", NoteID: note.ID, CreatorID: owner.ID})
			require.NoError(t, err)
			delete := &store.DeleteNoteShare{ID: &share.ID, NoteID: &note.ID, Policy: &store.NoteWritePolicy{ActorUserID: owner.ID}}
			var wantErr error
			var removeTrigger func()
			switch name {
			case "owner by uid":
				delete.ID, delete.UID = nil, &share.UID
			case "matching selectors":
				delete.UID = &share.UID
			case "instance admin":
				delete.Policy.ActorUserID = admin.ID
			case "archived note":
				require.NoError(t, ts.UpdateNote(ctx, &store.UpdateNote{ID: note.ID, RowStatus: new(store.Archived)}))
			case "unrelated actor":
				delete.Policy.ActorUserID = peer.ID
				wantErr = store.ErrNotePermissionDenied
			case "archived actor":
				_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: owner.ID, RowStatus: new(store.Archived)})
				require.NoError(t, err)
				wantErr = store.ErrNoteSpaceMembershipRequired
			case "missing actor":
				delete.Policy.ActorUserID = 99999
				wantErr = store.ErrNoteSpaceMembershipRequired
			case "demoted admin":
				delete.Policy.ActorUserID = admin.ID
				// The instance must keep another administrator for the demotion.
				_, err = createTestingUserWithRole(ctx, ts, "share-standing-admin", store.RoleAdmin)
				require.NoError(t, err)
				_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: admin.ID, Role: new(store.RoleUser)})
				require.NoError(t, err)
				wantErr = store.ErrNotePermissionDenied
			case "removed membership":
				require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, peer.ID))
				wantErr = store.ErrNoteSpaceMembershipRequired
			case "mismatched note":
				delete.NoteID = &other.ID
				wantErr = store.ErrNoteMutationConflict
			case "mismatched selectors":
				delete.UID = &keep.UID
				wantErr = store.ErrNoteMutationConflict
			case "missing share":
				delete.ID = new(int32(99999))
				wantErr = store.ErrNoteMutationConflict
			case "missing note":
				delete.NoteID = new(int32(99999))
				wantErr = store.ErrNoteMutationConflict
			case "database failure":
				removeTrigger = rejectStoreDelete(t, ts, "note_share")
			default:
			}
			before, err := ts.ListNoteShares(ctx, &store.FindNoteShare{})
			require.NoError(t, err)
			err = ts.DeleteNoteShare(ctx, delete)
			if wantErr != nil || removeTrigger != nil {
				if removeTrigger != nil {
					require.ErrorContains(t, err, "store test delete failure")
				} else {
					require.ErrorIs(t, err, wantErr)
				}
				after, err := ts.ListNoteShares(ctx, &store.FindNoteShare{})
				require.NoError(t, err)
				require.Equal(t, before, after)
				if removeTrigger != nil {
					removeTrigger()
					require.NoError(t, ts.DeleteNoteShare(ctx, delete), "failed transaction must release its locks")
				}
			} else {
				require.NoError(t, err)
				got, err := ts.GetNoteShare(ctx, &store.FindNoteShare{ID: &share.ID})
				require.NoError(t, err)
				require.Nil(t, got)
				require.ErrorIs(t, ts.DeleteNoteShare(ctx, delete), store.ErrNoteMutationConflict)
			}
			got, err := ts.GetNoteShare(ctx, &store.FindNoteShare{ID: &keep.ID})
			require.NoError(t, err)
			require.Equal(t, keep, got)
		})
	}
}

func TestNoteShareFiltersAndExpiryRoundTrip(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	peer, err := createTestingUserWithRole(ctx, ts, "share-peer", store.RoleUser)
	require.NoError(t, err)
	var shares []*store.NoteShare
	for i, user := range []*store.User{owner, peer} {
		note, err := ts.CreateNote(ctx, &store.Note{UID: user.Username, CreatorID: user.ID, Content: "note", Visibility: store.Private})
		require.NoError(t, err)
		var expiry *int64
		if i == 1 {
			expiry = new(time.Now().Add(-time.Hour).Unix())
		}
		share, err := ts.CreateNoteShare(ctx, &store.NoteShare{UID: user.Username, NoteID: note.ID, CreatorID: user.ID, ExpiresTs: expiry})
		require.NoError(t, err)
		shares = append(shares, share)
	}
	for _, share := range shares {
		for _, find := range []*store.FindNoteShare{
			{ID: &share.ID}, {UID: &share.UID}, {NoteID: &share.NoteID}, {CreatorID: &share.CreatorID},
			{ID: &share.ID, UID: &share.UID, NoteID: &share.NoteID, CreatorID: &share.CreatorID},
		} {
			got, err := ts.GetNoteShare(ctx, find)
			require.NoError(t, err)
			require.Equal(t, share, got)
			list, err := ts.ListNoteShares(ctx, find)
			require.NoError(t, err)
			require.Equal(t, []*store.NoteShare{share}, list, "store listing includes expired grants for management")
		}
	}
	find := &store.FindNoteShare{ID: &shares[0].ID, CreatorID: &peer.ID}
	got, err := ts.GetNoteShare(ctx, find)
	require.NoError(t, err)
	require.Nil(t, got)
	list, err := ts.ListNoteShares(ctx, find)
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = ts.CreateNoteShare(ctx, &store.NoteShare{UID: shares[0].UID, NoteID: shares[1].NoteID, CreatorID: peer.ID})
	require.Error(t, err, "duplicate bearer token must not replace its original grant")
	list, err = ts.ListNoteShares(ctx, &store.FindNoteShare{})
	require.NoError(t, err)
	require.Equal(t, shares, list)
}

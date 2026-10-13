package store_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestValidateNoteRelationEndpointReadDanglingPlacement(t *testing.T) {
	danglingSpaceID := int32(42)
	for _, visibility := range []store.Visibility{store.Public, store.Protected, store.Private} {
		t.Run(string(visibility), func(t *testing.T) {
			err := store.ValidateNoteRelationEndpointRead(&store.NoteRelationEndpointSnapshot{
				ActorUserID:          1,
				Actor:                store.NoteActorState{Active: true},
				EndpointID:           2,
				EndpointCreatorID:    1,
				EndpointRowStatus:    store.Normal,
				EndpointVisibility:   visibility,
				EndpointSpaceID:      &danglingSpaceID,
				EndpointSpaceExists:  false,
				EndpointMemberActive: false,
			})
			require.NoError(t, err, "a dangling placement must not override a non-Space audience")
		})
	}

	err := store.ValidateNoteRelationEndpointRead(&store.NoteRelationEndpointSnapshot{
		ActorUserID:          1,
		Actor:                store.NoteActorState{Active: true},
		EndpointID:           2,
		EndpointCreatorID:    2,
		EndpointRowStatus:    store.Normal,
		EndpointVisibility:   store.SpaceAudience,
		EndpointSpaceID:      &danglingSpaceID,
		EndpointSpaceExists:  false,
		EndpointMemberActive: true,
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict, "a Space audience must fail closed when its placement is dangling")
}

func TestValidateNoteWriteSnapshotRejectsDanglingSourcePlacement(t *testing.T) {
	danglingSpaceID := int32(42)
	err := store.ValidateNoteWriteSnapshot(
		&store.NoteWritePolicy{ActorUserID: 1},
		nil,
		&store.NoteWriteSnapshot{
			CreatorID:         1,
			RowStatus:         store.Normal,
			SpaceID:           &danglingSpaceID,
			Visibility:        store.Public,
			SourceSpaceExists: false,
		},
	)
	require.ErrorIs(t, err, store.ErrNoteSpaceNotWritable)
}

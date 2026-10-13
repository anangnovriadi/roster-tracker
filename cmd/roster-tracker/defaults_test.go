package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
	teststore "github.com/anangnovriadi/roster-tracker/store/test"
)

func TestApplyRosterTrackerDefaults(t *testing.T) {
	ctx := context.Background()
	s := teststore.NewTestingStore(ctx, t)

	require.NoError(t, applyRosterTrackerDefaults(ctx, s))
	// Running again on every start must not create a second admin.
	require.NoError(t, applyRosterTrackerDefaults(ctx, s))

	users, err := s.ListUsers(ctx, &store.FindUser{})
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "admin", users[0].Username)
	require.Equal(t, store.RoleAdmin, users[0].Role)
	require.Equal(t, "Administrator", users[0].Nickname)
	require.Equal(t, "admin@mail.com", users[0].Email)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(users[0].PasswordHash), []byte("12345678")))

	access, err := s.GetInstanceAccessSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC, access.AccessMode)
	general, err := s.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.True(t, general.DisallowUserRegistration)
}

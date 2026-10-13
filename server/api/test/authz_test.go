package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/anangnovriadi/roster-tracker/internal/random"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/server/api"
	"github.com/anangnovriadi/roster-tracker/server/auth"
)

// TestAuthorizerPrivateInstanceRegistration verifies that registration and other
// bootstrap methods stay reachable anonymously while unrelated public methods are
// gated. CreateUser enforces the instance registration settings in the service.
func TestAuthorizerPrivateInstanceRegistration(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))
	authorizer := api.NewAuthorizer(ts.Store, ts.Secret)

	const (
		createUser    = "/roster.api.UserService/CreateUser"
		signIn        = "/roster.api.AuthService/SignIn"
		listNotes     = "/roster.api.NoteService/ListNotes"
		listReactions = "/roster.api.NoteService/ListNoteReactions"
		listRelations = "/roster.api.NoteService/ListNoteRelations"
		getSharedNote = "/roster.api.NoteService/GetSharedNote"
	)

	// Anonymous request with no Authorization header resolves to no identity.
	require.Nil(t, authorizer.Authenticate(ctx, ""))

	// Registration and other bootstrap methods are allowed; browsing is still gated.
	require.NoError(t, authorizer.CheckAccess(ctx, createUser, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, signIn, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, getSharedNote, nil))
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listNotes, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listReactions, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listRelations, nil), api.ErrUnauthenticated)

	// Policy reads are fresh: changing to PUBLIC immediately opens browsing.
	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC))
	require.NoError(t, authorizer.CheckAccess(ctx, listNotes, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, listReactions, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, listRelations, nil))
	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))

	// Once a user exists, CreateUser remains reachable so UserService can enforce
	// disallow_user_registration and disallow_password_auth.
	_, err := ts.CreateHostUser(ctx, "host")
	require.NoError(t, err)
	require.NoError(t, authorizer.CheckAccess(ctx, createUser, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, signIn, nil))
}

// TestAuthorizerAccessTokenAlwaysWorksOnPrivateInstance verifies that a valid
// Personal Access Token authenticates through the Authorizer even on a private
// instance, and then passes the access check for an otherwise-gated method.
func TestAuthorizerAccessTokenAlwaysWorksOnPrivateInstance(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "pat-user")
	require.NoError(t, err)

	token := auth.GeneratePersonalAccessToken()
	require.NoError(t, ts.Store.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     random.UUID(),
		TokenHash:   auth.HashPersonalAccessToken(token),
		Description: "authz test PAT",
		CreatedAt:   timestamppb.Now(),
	}))

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))
	authorizer := api.NewAuthorizer(ts.Store, ts.Secret)

	result := authorizer.Authenticate(ctx, "Bearer "+token)
	require.NotNil(t, result)
	require.NotNil(t, result.User)
	require.Equal(t, user.ID, result.User.ID)

	// An authenticated caller passes access control even for a gated method.
	require.NoError(t, authorizer.CheckAccess(ctx, "/roster.api.NoteService/ListNotes", result))
}

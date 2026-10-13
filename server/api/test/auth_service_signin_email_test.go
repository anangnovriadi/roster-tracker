package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/server/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestSignInAcceptsUsernameOrEmail(t *testing.T) {
	ctx := context.Background()

	signIn := func(ts *TestService, identifier, password string) (*apipb.SignInResponse, error) {
		return ts.Service.SignIn(api.WithHeaderCarrier(ctx), &apipb.SignInRequest{
			Credentials: &apipb.SignInRequest_PasswordCredentials_{
				PasswordCredentials: &apipb.SignInRequest_PasswordCredentials{
					Username: identifier,
					Password: password,
				},
			},
		})
	}
	createUser := func(t *testing.T, ts *TestService, username, email, password string) *store.User {
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		require.NoError(t, err)
		user, err := ts.Store.CreateUser(ctx, &store.User{
			Username:     username,
			Role:         store.RoleUser,
			Email:        email,
			PasswordHash: string(passwordHash),
		})
		require.NoError(t, err)
		return user
	}

	t.Run("username", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user := createUser(t, ts, "alice", "alice@example.com", "password123")

		resp, err := signIn(ts, "alice", "password123")
		require.NoError(t, err)
		require.Equal(t, user.Username, resp.User.Username)
	})

	t.Run("email matches case-insensitively", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user := createUser(t, ts, "alice", "alice@example.com", "password123")

		resp, err := signIn(ts, " Alice@Example.COM ", "password123")
		require.NoError(t, err)
		require.Equal(t, user.Username, resp.User.Username)
		require.NotEmpty(t, resp.AccessToken)
	})

	t.Run("email with wrong password is rejected", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		createUser(t, ts, "alice", "alice@example.com", "password123")

		_, err := signIn(ts, "alice@example.com", "wrong-password")
		require.ErrorContains(t, err, "unmatched username and password")
	})

	t.Run("unknown or invalid email is rejected", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		createUser(t, ts, "alice", "alice@example.com", "password123")

		_, err := signIn(ts, "bob@example.com", "password123")
		require.ErrorContains(t, err, "unmatched username and password")
		_, err = signIn(ts, "not an @ address", "password123")
		require.ErrorContains(t, err, "unmatched username and password")
	})

	t.Run("legacy username containing @ wins over another account's email", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		legacy := createUser(t, ts, "shared@example.com", "", "legacy-pass")
		createUser(t, ts, "carol", "shared@example.com", "carol-pass")

		resp, err := signIn(ts, "shared@example.com", "legacy-pass")
		require.NoError(t, err)
		require.Equal(t, legacy.Username, resp.User.Username)
	})
}

package store_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/anangnovriadi/roster-tracker/internal/profile"
	"github.com/anangnovriadi/roster-tracker/store"
	"github.com/anangnovriadi/roster-tracker/store/db/sqlite"
)

func TestDemoSeedUsesDeploymentAuthenticationPolicy(t *testing.T) {
	ctx := context.Background()
	p := &profile.Profile{
		Demo:   true,
		Data:   t.TempDir(),
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "demo.db"),
	}
	driver, err := sqlite.NewDB(p)
	require.NoError(t, err)
	stores := store.New(driver, p)
	t.Cleanup(func() {
		require.NoError(t, stores.Close())
	})

	require.NoError(t, stores.Migrate(ctx))
	generalSetting, err := stores.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.False(t, generalSetting.DisallowPasswordAuth, "authentication policy must come from deployment configuration")
	require.False(t, generalSetting.DisallowUserRegistration, "SSO first-login provisioning must remain enabled")

	secretDir := t.TempDir()
	writeDeploymentIdentityProvider(t, filepath.Join(secretDir, "roster-tracker-idp-primary-sso.json"), "primary-sso", "Primary SSO", "secret")
	writeDeploymentGeneralSetting(t, filepath.Join(secretDir, "roster-tracker-instance-setting-general.json"), 0, true)
	require.NoError(t, stores.LoadDeploymentConfigurationDir(ctx, secretDir))
	generalSetting, err = stores.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.True(t, generalSetting.DisallowPasswordAuth)
	require.False(t, generalSetting.DisallowUserRegistration, "SSO first-login provisioning must remain enabled")
	provider, err := stores.GetIdentityProvider(ctx, &store.FindIdentityProvider{UID: new("primary-sso")})
	require.NoError(t, err)
	require.NotNil(t, provider)

	adminUsername := "steven"
	adminUser, err := stores.GetUser(ctx, &store.FindUser{Username: &adminUsername})
	require.NoError(t, err)
	require.NotNil(t, adminUser)
	require.Equal(t, store.RoleAdmin, adminUser.Role)
	require.Error(t, bcrypt.CompareHashAndPassword([]byte(adminUser.PasswordHash), []byte("demo")))
	adminCost, err := bcrypt.Cost([]byte(adminUser.PasswordHash))
	require.NoError(t, err)
	require.GreaterOrEqual(t, adminCost, 12)

	johnnyUsername := "johnny"
	johnnyUser, err := stores.GetUser(ctx, &store.FindUser{Username: &johnnyUsername})
	require.NoError(t, err)
	require.NotNil(t, johnnyUser)
	require.Error(t, bcrypt.CompareHashAndPassword([]byte(johnnyUser.PasswordHash), []byte("demo")))
	require.NotEqual(t, adminUser.PasswordHash, johnnyUser.PasswordHash)

	notes, err := stores.ListNotes(ctx, &store.FindNote{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(notes), 20)

	var hasPublic, hasProtected, hasLocation, hasNestedTag, hasMention bool
	for _, note := range notes {
		hasPublic = hasPublic || note.Visibility == store.Public
		hasProtected = hasProtected || note.Visibility == store.Protected
		hasLocation = hasLocation || note.Payload.GetLocation() != nil
		for _, tag := range note.Payload.GetTags() {
			hasNestedTag = hasNestedTag || strings.Contains(tag, "/")
		}
		hasMention = hasMention || strings.Contains(note.Content, "@steven")
	}
	require.True(t, hasPublic, "demo should include public notes")
	require.True(t, hasProtected, "demo should include protected notes")
	require.True(t, hasLocation, "demo should include location metadata")
	require.True(t, hasNestedTag, "demo should include nested tags")
	require.True(t, hasMention, "demo should include a mention")

	relations, err := stores.ListNoteRelations(ctx, &store.FindNoteRelation{})
	require.NoError(t, err)
	var hasComment, hasReference bool
	for _, relation := range relations {
		hasComment = hasComment || relation.Type == store.NoteRelationComment
		hasReference = hasReference || relation.Type == store.NoteRelationReference
	}
	require.True(t, hasComment, "demo should include comments")
	require.True(t, hasReference, "demo should include note references")

	reactions, err := stores.ListReactions(ctx, &store.FindReaction{})
	require.NoError(t, err)
	require.NotEmpty(t, reactions, "demo should include reactions")

	attachments, err := stores.ListAttachments(ctx, &store.FindAttachment{HasRelatedNote: true})
	require.NoError(t, err)
	require.NotEmpty(t, attachments, "demo should include a note attachment")
}

package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/anangnovriadi/roster-tracker/internal/profile"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
	storedb "github.com/anangnovriadi/roster-tracker/store/db"
)

// TestUpgradeFromBaselineRelease tests a database initialized by v0.31.0
// directly, independently of the pre-baseline intermediate upgrade test.
func TestUpgradeFromBaselineRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-based upgrade test in short mode")
	}
	skipIfContainerProviderUnavailable(t)

	ctx := context.Background()
	driver := getDriverFromEnv()
	cfg, hostDSN := prepareUpgradeFixture(t, driver, "0.31.0")
	container, err := StartNotesContainer(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if container != nil {
			_ = container.Terminate(ctx)
		}
	})
	require.NoError(t, container.Terminate(ctx))
	container = nil

	previous := NewTestingStoreWithDSN(ctx, t, driver, hostDSN)
	t.Cleanup(func() { _ = previous.Close() })
	baseline, err := previous.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, "0.31.8", baseline.SchemaVersion)

	requireUpgradePreservesData(ctx, t, previous, &profile.Profile{
		Data: t.TempDir(), DSN: hostDSN, Driver: driver, Version: "26.10",
	})
}

// TestUpgradeFromCalendarRelease uses local store fixtures until a published
// CalVer release is available. Application versions do not advance the schema:
// only migrations do.
func TestUpgradeFromCalendarRelease(t *testing.T) {
	for _, tc := range []struct{ previous, candidate string }{
		{previous: "26.09", candidate: "26.10"},
		{previous: "26.10", candidate: "26.10.1"},
		{previous: "26.10-rc.1", candidate: "26.10"},
		{previous: "26.12.9", candidate: "27.01"},
	} {
		t.Run(tc.previous+"-to-"+tc.candidate, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			previousProfile := getTestingProfileForDriver(t, getDriverFromEnv())
			previousProfile.Version = tc.previous
			previousDriver, err := storedb.NewDBDriver(previousProfile)
			require.NoError(t, err)
			previous := store.New(previousDriver, previousProfile)
			t.Cleanup(func() { _ = previous.Close() })
			require.NoError(t, previous.Migrate(ctx))
			baseline, err := previous.GetInstanceBasicSetting(ctx)
			require.NoError(t, err)
			currentSchema, err := previous.GetCurrentSchemaVersion()
			require.NoError(t, err)
			require.Equal(t, currentSchema, baseline.SchemaVersion, "application CalVer must not become the database schema version")

			candidateProfile := *previousProfile
			candidateProfile.Version = tc.candidate
			requireUpgradePreservesData(ctx, t, previous, &candidateProfile)
		})
	}
}

// requireUpgradePreservesData closes the source store and opens the same
// database with the candidate profile, then checks upgrade and restart behavior.
func requireUpgradePreservesData(ctx context.Context, t *testing.T, previous *store.Store, candidateProfile *profile.Profile) {
	t.Helper()
	user, err := createTestingHostUser(ctx, previous)
	require.NoError(t, err)
	note, err := previous.CreateNote(ctx, &store.Note{
		UID: "upgrade-sentinel", CreatorID: user.ID, Content: "#upgrade\n- [ ] preserved task", Visibility: store.Private, Pinned: true,
	})
	require.NoError(t, err)
	note, err = previous.GetNote(ctx, &store.FindNote{UID: &note.UID})
	require.NoError(t, err)
	view, err := previous.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID, Key: storepb.UserSetting_NOTE_VIEWS,
		Value: &storepb.UserSetting_NoteViews{NoteViews: &storepb.NoteViewsUserSetting{
			NoteViews: []*storepb.NoteViewsUserSetting_NoteView{{Id: "upgrade", Title: "Upgrade sentinel", Filter: "has_incomplete_tasks"}},
		}},
	})
	require.NoError(t, err)
	access, err := previous.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_ACCESS,
		Value: &storepb.InstanceSetting_AccessSetting{AccessSetting: &storepb.InstanceAccessSetting{
			AccessMode: storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC,
		}},
	})
	require.NoError(t, err)
	require.NoError(t, previous.Close())

	verify := func(candidate *store.Store) {
		t.Helper()
		currentSchema, err := candidate.GetCurrentSchemaVersion()
		require.NoError(t, err)
		basic, err := candidate.GetInstanceBasicSetting(ctx)
		require.NoError(t, err)
		require.Equal(t, currentSchema, basic.SchemaVersion)
		preservedUser, err := candidate.GetUser(ctx, &store.FindUser{ID: &user.ID})
		require.NoError(t, err)
		require.Equal(t, user, preservedUser)
		preservedNote, err := candidate.GetNote(ctx, &store.FindNote{UID: &note.UID})
		require.NoError(t, err)
		require.Equal(t, note, preservedNote)
		preservedView, err := candidate.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_NOTE_VIEWS})
		require.NoError(t, err)
		require.True(t, proto.Equal(view, preservedView), "saved Views must survive the upgrade")
		preservedAccess, err := candidate.GetInstanceAccessSetting(ctx)
		require.NoError(t, err)
		require.True(t, proto.Equal(access.GetAccessSetting(), preservedAccess), "explicit access mode must not be reset")
		requireCreatorIndexes(ctx, t, candidate, candidateProfile.Driver)
	}

	driver, err := storedb.NewDBDriver(candidateProfile)
	require.NoError(t, err)
	candidate := store.New(driver, candidateProfile)
	t.Cleanup(func() { _ = candidate.Close() })
	require.NoError(t, candidate.Migrate(ctx))
	verify(candidate)
	written, err := candidate.CreateNote(ctx, &store.Note{
		UID: "after-upgrade", CreatorID: user.ID, Content: "created after upgrade", Visibility: store.Private,
	})
	require.NoError(t, err, "upgraded database must remain writable")
	require.NoError(t, candidate.Close())

	restartedDriver, err := storedb.NewDBDriver(candidateProfile)
	require.NoError(t, err)
	restarted := store.New(restartedDriver, candidateProfile)
	t.Cleanup(func() { _ = restarted.Close() })
	require.NoError(t, restarted.Migrate(ctx), "restart after upgrade must be idempotent")
	verify(restarted)
	persisted, err := restarted.GetNote(ctx, &store.FindNote{UID: &written.UID})
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, written.Content, persisted.Content, "post-upgrade writes must survive restart")
}

// prepareUpgradeFixture returns the container config needed to bootstrap a real
// schema for the given Roster Tracker version, plus the DSN the host uses to reach it.
func prepareUpgradeFixture(t *testing.T, driver, version string) (NotesContainerConfig, string) {
	t.Helper()

	switch driver {
	case "sqlite":
		dataDir := t.TempDir()
		return NotesContainerConfig{
			Version: version,
			Driver:  driver,
			DataDir: dataDir,
		}, fmt.Sprintf("%s/notes_prod.db", dataDir)
	case "mysql":
		hostDSN := GetMySQLDSN(t)
		containerDSN, err := getContainerDSN(driver, hostDSN)
		require.NoError(t, err)
		return NotesContainerConfig{
			Version: version,
			Driver:  driver,
			DSN:     containerDSN,
		}, hostDSN
	case "postgres":
		hostDSN := GetPostgresDSN(t)
		containerDSN, err := getContainerDSN(driver, hostDSN)
		require.NoError(t, err)
		return NotesContainerConfig{
			Version: version,
			Driver:  driver,
			DSN:     containerDSN,
		}, hostDSN
	default:
		t.Fatalf("unsupported driver: %s", driver)
		return NotesContainerConfig{}, ""
	}
}

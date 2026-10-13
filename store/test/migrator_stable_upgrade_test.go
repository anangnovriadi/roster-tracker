package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// TestUpgradeThroughBaselineRelease verifies the required intermediate upgrade
// using released binaries, including data transformations owned by v0.31.0.
func TestUpgradeThroughBaselineRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container-based upgrade test in short mode")
	}
	skipIfContainerProviderUnavailable(t)

	ctx := context.Background()
	driver := getDriverFromEnv()

	cfg, hostDSN := prepareUpgradeFixture(t, driver, PreBaselineNotesVersion)
	t.Logf("Starting Roster Tracker %s container for %s schema bootstrap...", cfg.Version, driver)
	container, err := StartNotesContainer(ctx, cfg)
	require.NoError(t, err, "failed to start notes %s container", PreBaselineNotesVersion)
	t.Cleanup(func() {
		if container != nil {
			_ = container.Terminate(ctx)
		}
	})

	legacyStore := NewTestingStoreWithDSN(ctx, t, driver, hostDSN)
	require.Eventually(t, func() bool {
		setting, err := legacyStore.GetInstanceBasicSetting(ctx)
		return err == nil && setting != nil && setting.SchemaVersion != ""
	}, 45*time.Second, 500*time.Millisecond, "previous stable should initialize its schema")

	settingBeforeUpgrade, err := legacyStore.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	t.Logf("Schema version written by %s: %s", PreBaselineNotesVersion, settingBeforeUpgrade.SchemaVersion)

	require.NoError(t, container.Terminate(ctx), "failed to stop notes %s container", PreBaselineNotesVersion)
	container = nil
	require.NoError(t, legacyStore.Close())

	// Seed through the store API where the schema is shared by v0.30.0 and the
	// current build. The legacy SHORTCUTS rows predate the current proto, so
	// they are seeded with raw SQL exactly as the previous release stored them.
	seedStore := NewTestingStoreWithDSN(ctx, t, driver, hostDSN)

	renamedUser, err := createTestingUserWithRole(ctx, seedStore, "legacyviews", store.RoleUser)
	require.NoError(t, err)
	keepsOwnUser, err := createTestingUserWithRole(ctx, seedStore, "keepsown", store.RoleUser)
	require.NoError(t, err)

	settingKeyColumn := "key"
	if driver == "mysql" {
		settingKeyColumn = "`key`"
	}
	// Bound rather than inlined: MySQL treats backslash as an escape character
	// inside string literals, so an inlined \" would collapse and corrupt the
	// seeded JSON.
	insertSetting := fmt.Sprintf("INSERT INTO user_setting (user_id, %[1]s, value) VALUES (?, ?, ?)", settingKeyColumn)
	if driver == "postgres" {
		insertSetting = fmt.Sprintf("INSERT INTO user_setting (user_id, %[1]s, value) VALUES ($1, $2, $3)", settingKeyColumn)
	}
	db := seedStore.GetDriver().GetDB()
	insertNote := "INSERT INTO note (uid, creator_id, content, payload) VALUES (?, ?, ?, '{}')"
	if driver == "postgres" {
		insertNote = "INSERT INTO note (uid, creator_id, content, payload) VALUES ($1, $2, $3, '{}')"
	}
	_, err = db.ExecContext(ctx, insertNote, "before-baseline", renamedUser.ID, "note from v0.30.0")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, insertSetting, renamedUser.ID, "SHORTCUTS",
		`{"shortcuts":[{"id":"legacy","title":"legacy shortcut","filter":"tag in [\"legacy\"]"}]}`)
	require.NoError(t, err, "should seed a legacy SHORTCUTS setting")
	_, err = db.ExecContext(ctx, insertSetting, keepsOwnUser.ID, "SHORTCUTS",
		`{"shortcuts":[{"id":"stale","title":"stale","filter":"tag in [\"stale\"]"}]}`)
	require.NoError(t, err, "should seed a conflicting legacy SHORTCUTS setting")

	// One user already has a NOTE_VIEWS setting; the migration must not clobber it.
	_, err = seedStore.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: keepsOwnUser.ID,
		Key:    storepb.UserSetting_NOTE_VIEWS,
		Value: &storepb.UserSetting_NoteViews{
			NoteViews: &storepb.NoteViewsUserSetting{
				NoteViews: []*storepb.NoteViewsUserSetting_NoteView{
					{Id: "own", Title: "own view", Filter: `tag in ["own"]`},
				},
			},
		},
	})
	require.NoError(t, err, "should seed a pre-existing NOTE_VIEWS setting")

	// The user whose setting should be renamed must not have one yet.
	preExisting, err := seedStore.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &renamedUser.ID,
		Key:    storepb.UserSetting_NOTE_VIEWS,
	})
	require.NoError(t, err)
	require.Nil(t, preExisting, "the seeded user should not have a NOTE_VIEWS setting before the upgrade")

	// The current binary must refuse the old schema before modifying settings.
	before, err := seedStore.GetDriver().ListInstanceSettings(ctx, &store.FindInstanceSetting{})
	require.NoError(t, err)
	require.ErrorContains(t, seedStore.Migrate(ctx), "First upgrade to v0.31.0")
	after, err := seedStore.GetDriver().ListInstanceSettings(ctx, &store.FindInstanceSetting{})
	require.NoError(t, err)
	require.ElementsMatch(t, before, after)
	var preservedContent string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT content FROM note WHERE uid = 'before-baseline'").Scan(&preservedContent))
	require.Equal(t, "note from v0.30.0", preservedContent)
	require.NoError(t, seedStore.Close())

	// Only the released intermediate binary carries the historical migrations.
	cfg.Version = "0.31.0"
	container, err = StartNotesContainer(ctx, cfg)
	require.NoError(t, err, "failed to start the required intermediate release")
	require.NoError(t, container.Terminate(ctx))
	container = nil

	// Upgrade with current code.
	ts := NewTestingStoreWithDSN(ctx, t, driver, hostDSN)
	defer ts.Close()
	baseline, err := ts.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, "0.31.8", baseline.SchemaVersion, "v0.31.0 must establish the fixed baseline")
	require.NoError(t, ts.Migrate(ctx), "upgrade from v0.31.0 should succeed for %s", driver)

	currentVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)
	upgradedSetting, err := ts.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, currentVersion, upgradedSetting.SchemaVersion, "schema version should advance")
	noteUID := "before-baseline"
	preservedNote, err := ts.GetNote(ctx, &store.FindNote{UID: &noteUID})
	require.NoError(t, err)
	require.NotNil(t, preservedNote)
	require.Equal(t, "note from v0.30.0", preservedNote.Content)
	require.Equal(t, renamedUser.ID, preservedNote.CreatorID)

	// The legacy SHORTCUTS setting is now readable as NOTE_VIEWS.
	renamed, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &renamedUser.ID,
		Key:    storepb.UserSetting_NOTE_VIEWS,
	})
	require.NoError(t, err)
	require.NotNil(t, renamed, "legacy shortcuts should be renamed to note views")
	require.Len(t, renamed.GetNoteViews().GetNoteViews(), 1)
	require.Equal(t, "legacy", renamed.GetNoteViews().GetNoteViews()[0].GetId())
	require.Equal(t, "legacy shortcut", renamed.GetNoteViews().GetNoteViews()[0].GetTitle())
	require.Equal(t, `tag in ["legacy"]`, renamed.GetNoteViews().GetNoteViews()[0].GetFilter())

	// The user with their own setting keeps it untouched.
	kept, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &keepsOwnUser.ID,
		Key:    storepb.UserSetting_NOTE_VIEWS,
	})
	require.NoError(t, err)
	require.NotNil(t, kept)
	require.Len(t, kept.GetNoteViews().GetNoteViews(), 1)
	require.Equal(t, "own", kept.GetNoteViews().GetNoteViews()[0].GetId(), "existing note views should not be overwritten")

	// Re-running the upgrade must stay a no-op, which is what a container
	// restart on an already-upgraded volume does.
	require.NoError(t, ts.Migrate(ctx), "re-running the upgrade should be idempotent")

	// The upgraded database must still accept writes.
	postUpgradeNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "post-stable-upgrade-note",
		CreatorID:  renamedUser.ID,
		Content:    "created after upgrading from previous stable",
		Visibility: store.Private,
	})
	require.NoError(t, err)
	require.Equal(t, "created after upgrading from previous stable", postUpgradeNote.Content)
}

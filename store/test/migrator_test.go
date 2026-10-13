package test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
	storedb "github.com/anangnovriadi/roster-tracker/store/db"
)

type delayedInstanceSettingCreateDriver struct {
	store.Driver
	entered chan<- struct{}
	release <-chan struct{}
}

func (d *delayedInstanceSettingCreateDriver) CreateInstanceSettingIfNotExists(ctx context.Context, create *store.InstanceSetting) (bool, error) {
	d.entered <- struct{}{}
	<-d.release
	return d.Driver.CreateInstanceSettingIfNotExists(ctx, create)
}

func requireQueryError(ctx context.Context, t *testing.T, db *sql.DB, query, message string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, query)
	if rows != nil {
		defer rows.Close()
		require.NoError(t, rows.Err())
	}
	require.Error(t, err, message)
}

// TestFreshInstall verifies that LATEST.sql applies correctly on a fresh database.
// This is essentially what NewTestingStore already does, but we make it explicit.
func TestFreshInstall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// NewTestingStore creates a fresh database and runs Migrate()
	// which applies LATEST.sql for uninitialized databases
	ts := NewTestingStore(ctx, t)

	// Verify migration completed successfully
	currentSchemaVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, "26.10.1", currentSchemaVersion, "fresh install should record the latest migration")

	// Verify we can read instance settings (basic sanity check)
	instanceSetting, err := ts.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, currentSchemaVersion, instanceSetting.SchemaVersion)

	// The fresh schema supports note-local Space placement without adding a
	// canonical thread shape. COMMENT remains an ordinary relation row.
	driver := getDriverFromEnv()
	insertSpace := "INSERT INTO space (id, uid, title, description, payload) VALUES (?, ?, ?, ?, '{}')"
	insertNote := "INSERT INTO note (id, uid, creator_id, content, visibility, payload, space_id) VALUES (?, ?, ?, ?, ?, ?, ?)"
	insertRelation := "INSERT INTO note_relation (note_id, related_note_id, type) VALUES (?, ?, ?)"
	if driver == "postgres" {
		insertSpace = "INSERT INTO space (id, uid, title, description, payload) VALUES ($1, $2, $3, $4, '{}')"
		insertNote = "INSERT INTO note (id, uid, creator_id, content, visibility, payload, space_id) VALUES ($1, $2, $3, $4, $5, $6, $7)"
		insertRelation = "INSERT INTO note_relation (note_id, related_note_id, type) VALUES ($1, $2, $3)"
	}
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, insertSpace, 900001, "fresh-space", "Fresh Space", "schema fixture")
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, insertNote, 900001, "fresh-context", 1, "context", store.Public, `{}`, nil)
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, insertNote, 900002, "fresh-comment", 1, "comment", store.SpaceAudience, `{}`, 900001)
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, insertRelation, 900002, 900001, store.NoteRelationComment)
	require.NoError(t, err)

	var relationCount int
	require.NoError(t, ts.GetDriver().GetDB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM note_relation WHERE note_id = 900002 AND related_note_id = 900001 AND type = 'COMMENT'",
	).Scan(&relationCount))
	require.Equal(t, 1, relationCount)

	requireQueryError(ctx, t, ts.GetDriver().GetDB(), "SELECT parent_note_id, root_note_id FROM note LIMIT 0", "fresh note schema must not contain canonical-root columns")
	requireQueryError(ctx, t, ts.GetDriver().GetDB(), "SELECT row_status FROM space LIMIT 0", "fresh Space schema has no archived state")
	requireCreatorIndexes(ctx, t, ts, driver)
}

// requireCreatorIndexes checks the indexes that usage limit checks rely on to
// count a user's notes and sum a user's attachment sizes.
func requireCreatorIndexes(ctx context.Context, t *testing.T, ts *store.Store, driver string) {
	t.Helper()
	for _, indexName := range []string{"idx_note_creator_id", "idx_attachment_creator_id"} {
		var query string
		switch driver {
		case "sqlite":
			query = "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?"
		case "mysql":
			query = "SELECT COUNT(DISTINCT INDEX_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND INDEX_NAME = ?"
		case "postgres":
			query = "SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1"
		default:
			t.Fatalf("unsupported driver: %s", driver)
		}
		var count int
		require.NoError(t, ts.GetDriver().GetDB().QueryRowContext(ctx, query, indexName).Scan(&count))
		require.Equal(t, 1, count, "index %s must exist", indexName)
	}
}

// TestMigrationReRun verifies that re-running the migration on an already
// migrated database does not fail or cause issues. This simulates a
// scenario where the server is restarted.
func TestMigrationReRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Use the shared testing store which already runs migrations on init
	ts := NewTestingStore(ctx, t)

	// Get current version
	initialVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)

	// Manually trigger migration again
	err = ts.Migrate(ctx)
	require.NoError(t, err, "re-running migration should not fail")

	// Verify version hasn't changed (or at least is valid)
	finalVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, initialVersion, finalVersion, "version should match after re-run")
}

// TestMigrationWithData verifies that migration preserves data integrity.
// Creates data, then re-runs migration and verifies data is still accessible.
func TestMigrationWithData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	// Create a user and note before re-running migration
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err, "should create user")

	originalNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        "migration-data-test",
		CreatorID:  user.ID,
		Content:    "Data before migration re-run",
		Visibility: store.Public,
	})
	require.NoError(t, err, "should create note")

	// Re-run migration
	err = ts.Migrate(ctx)
	require.NoError(t, err, "re-running migration should not fail")

	// Verify data is still accessible
	note, err := ts.GetNote(ctx, &store.FindNote{UID: &originalNote.UID})
	require.NoError(t, err, "should retrieve note after migration")
	require.Equal(t, "Data before migration re-run", note.Content, "note content should be preserved")
}

// TestMigrationMultipleReRuns verifies that migration is idempotent
// even when run multiple times in succession.
func TestMigrationMultipleReRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	// Get initial version
	initialVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)

	// Run migration multiple times
	for i := range 3 {
		err = ts.Migrate(ctx)
		require.NoError(t, err, "migration run %d should not fail", i+1)
	}

	// Verify version is still correct
	finalVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, initialVersion, finalVersion, "version should remain unchanged after multiple re-runs")
}

func TestConcurrentInstanceAccessInitializationKeepsFirstInsert(t *testing.T) {
	ctx := context.Background()
	driverName := getDriverFromEnv()
	baseProfile := getTestingProfileForDriver(t, driverName)

	baseDriver, err := storedb.NewDBDriver(baseProfile)
	require.NoError(t, err)
	baseStore := store.New(baseDriver, baseProfile)
	require.NoError(t, baseStore.Migrate(ctx))
	require.NoError(t, baseStore.DeleteInstanceSetting(ctx, &store.DeleteInstanceSetting{
		Name: storepb.InstanceSettingKey_ACCESS.String(),
	}))
	require.NoError(t, baseStore.Close())

	publicProfile := *baseProfile
	publicProfile.InstanceURL = "https://public.example.com"
	publicDriver, err := storedb.NewDBDriver(&publicProfile)
	require.NoError(t, err)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	delayedPublicDriver := &delayedInstanceSettingCreateDriver{
		Driver:  publicDriver,
		entered: entered,
		release: release,
	}
	publicStore := store.New(delayedPublicDriver, &publicProfile)
	defer publicStore.Close()

	privateProfile := *baseProfile
	privateProfile.InstanceURL = ""
	privateDriver, err := storedb.NewDBDriver(&privateProfile)
	require.NoError(t, err)
	privateStore := store.New(privateDriver, &privateProfile)
	defer privateStore.Close()

	publicMigration := make(chan error, 1)
	go func() {
		publicMigration <- publicStore.Migrate(ctx)
	}()
	<-entered
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	// The private instance reaches the database first. The delayed public
	// initializer must lose the unique-key race without overwriting it.
	require.NoError(t, privateStore.Migrate(ctx))
	close(release)
	released = true
	require.NoError(t, <-publicMigration)

	setting, err := privateStore.GetStoredInstanceSetting(ctx, &store.FindInstanceSetting{
		Name: storepb.InstanceSettingKey_ACCESS.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, setting)
	require.Equal(t, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE, setting.GetAccessSetting().AccessMode)

	// Re-running either initializer is also a no-op once ACCESS is persisted.
	require.NoError(t, publicStore.Migrate(ctx))
	setting, err = publicStore.GetStoredInstanceSetting(ctx, &store.FindInstanceSetting{
		Name: storepb.InstanceSettingKey_ACCESS.String(),
	})
	require.NoError(t, err)
	require.Equal(t, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE, setting.GetAccessSetting().AccessMode)
}

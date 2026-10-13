package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestValidateSQLiteNoteCreatePreservesQueryErrors(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`
		CREATE TABLE user (id INTEGER PRIMARY KEY, row_status TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'USER');
		CREATE TABLE space (id INTEGER PRIMARY KEY);
		CREATE TABLE space_member (space_id INTEGER, user_id INTEGER, status TEXT, role TEXT);
		INSERT INTO user (id, row_status) VALUES (1, 'NORMAL'), (2, 'ARCHIVED');
	`)
	require.NoError(t, err)

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	err = validateSQLiteNoteCreate(canceledContext, db, &store.Note{CreatorID: 1})
	require.ErrorIs(t, err, context.Canceled)

	err = validateSQLiteNoteCreate(context.Background(), db, &store.Note{CreatorID: 99})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)
	err = validateSQLiteNoteCreate(context.Background(), db, &store.Note{CreatorID: 2})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)

	err = validateSQLiteNoteSpaceMember(canceledContext, db, 1, 1)
	require.ErrorIs(t, err, context.Canceled)
	err = validateSQLiteNoteSpaceMember(context.Background(), db, 99, 1)
	require.ErrorIs(t, err, store.ErrNoteSpaceNotWritable)
}

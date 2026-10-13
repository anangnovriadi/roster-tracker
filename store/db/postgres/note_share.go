package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) CreateNoteShare(ctx context.Context, create *store.NoteShare) (*store.NoteShare, error) {
	if create.Policy == nil {
		return createPostgresNoteShare(ctx, d.db, create)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := validatePostgresNoteWritePolicy(ctx, tx, create.NoteID, create.Policy, nil); err != nil {
		return nil, err
	}
	result, err := createPostgresNoteShare(ctx, tx, create)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

type postgresNoteShareCreator interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func createPostgresNoteShare(ctx context.Context, executor postgresNoteShareCreator, create *store.NoteShare) (*store.NoteShare, error) {
	fields := []string{"uid", "note_id", "creator_id"}
	args := []any{create.UID, create.NoteID, create.CreatorID}

	if create.ExpiresTs != nil {
		fields = append(fields, "expires_ts")
		args = append(args, *create.ExpiresTs)
	}

	stmt := "INSERT INTO note_share (" + strings.Join(fields, ", ") + ") VALUES (" + placeholders(len(args)) + ") RETURNING id, created_ts"
	if err := executor.QueryRowContext(ctx, stmt, args...).Scan(
		&create.ID,
		&create.CreatedTs,
	); err != nil {
		return nil, err
	}
	return create, nil
}

func (d *DB) ListNoteShares(ctx context.Context, find *store.FindNoteShare) ([]*store.NoteShare, error) {
	where, args := []string{"1 = 1"}, []any{}

	if find.ID != nil {
		where, args = append(where, "id = "+placeholder(len(args)+1)), append(args, *find.ID)
	}
	if find.UID != nil {
		where, args = append(where, "uid = "+placeholder(len(args)+1)), append(args, *find.UID)
	}
	if find.NoteID != nil {
		where, args = append(where, "note_id = "+placeholder(len(args)+1)), append(args, *find.NoteID)
	}
	if find.CreatorID != nil {
		where, args = append(where, "creator_id = "+placeholder(len(args)+1)), append(args, *find.CreatorID)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT
			id,
			uid,
			note_id,
			creator_id,
			created_ts,
			expires_ts
		FROM note_share
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY id ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*store.NoteShare{}
	for rows.Next() {
		ms := &store.NoteShare{}
		if err := rows.Scan(
			&ms.ID,
			&ms.UID,
			&ms.NoteID,
			&ms.CreatorID,
			&ms.CreatedTs,
			&ms.ExpiresTs,
		); err != nil {
			return nil, err
		}
		list = append(list, ms)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (d *DB) GetNoteShare(ctx context.Context, find *store.FindNoteShare) (*store.NoteShare, error) {
	where, args := []string{"1 = 1"}, []any{}

	if find.ID != nil {
		where, args = append(where, "id = "+placeholder(len(args)+1)), append(args, *find.ID)
	}
	if find.UID != nil {
		where, args = append(where, "uid = "+placeholder(len(args)+1)), append(args, *find.UID)
	}
	if find.NoteID != nil {
		where, args = append(where, "note_id = "+placeholder(len(args)+1)), append(args, *find.NoteID)
	}
	if find.CreatorID != nil {
		where, args = append(where, "creator_id = "+placeholder(len(args)+1)), append(args, *find.CreatorID)
	}

	ms := &store.NoteShare{}
	if err := d.db.QueryRowContext(ctx, `
		SELECT
			id,
			uid,
			note_id,
			creator_id,
			created_ts,
			expires_ts
		FROM note_share
		WHERE `+strings.Join(where, " AND ")+`
		LIMIT 1`,
		args...,
	).Scan(
		&ms.ID,
		&ms.UID,
		&ms.NoteID,
		&ms.CreatorID,
		&ms.CreatedTs,
		&ms.ExpiresTs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return ms, nil
}

func (d *DB) DeleteNoteShare(ctx context.Context, delete *store.DeleteNoteShare) error {
	if delete.Policy != nil {
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := validatePostgresNoteWritePolicy(ctx, tx, *delete.NoteID, delete.Policy, nil); err != nil {
			return err
		}
		where, args := postgresNoteShareDeleteWhere(delete)
		where = append(where, "note_id = "+placeholder(len(args)+1))
		args = append(args, *delete.NoteID)
		result, err := tx.ExecContext(ctx, "DELETE FROM note_share WHERE "+strings.Join(where, " AND "), args...)
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil {
			return err
		} else if rows != 1 {
			return store.ErrNoteMutationConflict
		}
		return tx.Commit()
	}
	where, args := postgresNoteShareDeleteWhere(delete)
	_, err := d.db.ExecContext(ctx, "DELETE FROM note_share WHERE "+strings.Join(where, " AND "), args...)
	return err
}

func postgresNoteShareDeleteWhere(delete *store.DeleteNoteShare) ([]string, []any) {
	where, args := []string{"1 = 1"}, []any{}
	if delete.ID != nil {
		where, args = append(where, "id = "+placeholder(len(args)+1)), append(args, *delete.ID)
	}
	if delete.UID != nil {
		where, args = append(where, "uid = "+placeholder(len(args)+1)), append(args, *delete.UID)
	}
	return where, args
}

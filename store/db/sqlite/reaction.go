package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) UpsertReaction(ctx context.Context, upsert *store.Reaction) (*store.Reaction, error) {
	if upsert.Policy == nil {
		return upsertSQLiteReaction(ctx, d.db, upsert)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin reaction upsert transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateSQLiteReactionWritePolicy(ctx, tx, upsert); err != nil {
		return nil, err
	}
	reaction, err := upsertSQLiteReaction(ctx, tx, upsert)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit reaction upsert transaction")
	}
	return reaction, nil
}

type sqliteReactionUpsertQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func upsertSQLiteReaction(ctx context.Context, querier sqliteReactionUpsertQuerier, upsert *store.Reaction) (*store.Reaction, error) {
	if err := querier.QueryRowContext(ctx, `
		INSERT INTO reaction (creator_id, note_id, reaction_type)
		SELECT ?, note.id, ?
		FROM note
		WHERE note.id = ?
		RETURNING id, created_ts
	`, upsert.CreatorID, upsert.ReactionType, upsert.NoteID).Scan(
		&upsert.ID,
		&upsert.CreatedTs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Wrap(store.ErrReactionNoteNotFound, "failed to create reaction")
		}
		return nil, err
	}

	return upsert, nil
}

func (d *DB) ListReactions(ctx context.Context, find *store.FindReaction) ([]*store.Reaction, error) {
	where, args := []string{"1 = 1"}, []any{}

	if find.ID != nil {
		where, args = append(where, "id = ?"), append(args, *find.ID)
	}
	if find.CreatorID != nil {
		where, args = append(where, "creator_id = ?"), append(args, *find.CreatorID)
	}
	if find.NoteID != nil {
		where, args = append(where, "note_id = ?"), append(args, *find.NoteID)
	}
	if len(find.NoteIDList) > 0 {
		placeholders := make([]string, 0, len(find.NoteIDList))
		for _, id := range find.NoteIDList {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		where = append(where, "note_id IN ("+strings.Join(placeholders, ",")+")")
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT
			id,
			created_ts,
			creator_id,
			note_id,
			reaction_type
		FROM reaction
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY id ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*store.Reaction{}
	for rows.Next() {
		reaction := &store.Reaction{}
		if err := rows.Scan(
			&reaction.ID,
			&reaction.CreatedTs,
			&reaction.CreatorID,
			&reaction.NoteID,
			&reaction.ReactionType,
		); err != nil {
			return nil, err
		}
		list = append(list, reaction)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (d *DB) GetReaction(ctx context.Context, find *store.FindReaction) (*store.Reaction, error) {
	list, err := d.ListReactions(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	return list[0], nil
}

func (d *DB) DeleteReaction(ctx context.Context, delete *store.DeleteReaction) error {
	if delete.ActorUserID != nil {
		return d.deleteReactionAsCreator(ctx, delete)
	}
	where, args := []string{}, []any{}
	if delete.ID != nil {
		where, args = append(where, "`id` = ?"), append(args, *delete.ID)
	}
	if delete.NoteID != nil {
		where, args = append(where, "`note_id` = ?"), append(args, *delete.NoteID)
	}
	if len(where) == 0 {
		return nil
	}

	_, err := d.db.ExecContext(ctx, "DELETE FROM `reaction` WHERE "+strings.Join(where, " AND "), args...)
	return err
}

func (d *DB) deleteReactionAsCreator(ctx context.Context, delete *store.DeleteReaction) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin authorized reaction delete transaction")
	}
	defer func() { _ = tx.Rollback() }()
	actorIsAdmin := false
	if delete.Policy != nil {
		participation, err := validateSQLiteReactionDeletePolicy(ctx, tx, &store.Reaction{
			CreatorID: *delete.ActorUserID,
			NoteID:    *delete.NoteID,
			Policy:    delete.Policy,
		})
		if err != nil {
			return err
		}
		actorIsAdmin = participation.Actor.Admin
	}

	var creatorID, noteID int32
	if err := tx.QueryRowContext(ctx, "SELECT creator_id, note_id FROM reaction WHERE id = ?", *delete.ID).Scan(&creatorID, &noteID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return errors.Wrap(err, "failed to read reaction for deletion")
	}
	// An instance administrator may withdraw any reaction on the named note.
	if (creatorID != *delete.ActorUserID && !actorIsAdmin) || (delete.NoteID != nil && noteID != *delete.NoteID) {
		return store.ErrReactionPermissionDenied
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM reaction WHERE id = ?", *delete.ID); err != nil {
		return errors.Wrap(err, "failed to delete reaction")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit authorized reaction delete transaction")
	}
	return nil
}

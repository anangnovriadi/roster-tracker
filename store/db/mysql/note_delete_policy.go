package mysql

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) DeleteNoteWithPolicy(ctx context.Context, delete *store.DeleteNoteWithPolicy) (*store.DeleteNoteWithPolicyResult, error) {
	// Later subtree and attachment reads must see writers that committed before
	// their note locks became available.
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin note delete transaction")
	}
	defer func() { _ = tx.Rollback() }()

	actor, err := requireMySQLActiveNoteActor(ctx, tx, delete.ActorUserID)
	if err != nil {
		return nil, err
	}

	var creatorID int32
	var rowStatus store.RowStatus
	var visibility store.Visibility
	var space sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT creator_id, row_status, visibility, space_id FROM note WHERE id = ? FOR UPDATE", delete.NoteID).Scan(
		&creatorID, &rowStatus, &visibility, &space,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNoteMutationConflict
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to read note")
	}
	if creatorID != delete.ActorUserID && !actor.Admin {
		return nil, store.ErrNotePermissionDenied
	}
	spaceID := store.NullInt32Pointer(space)
	spaceExists := false
	actorMember := false
	if spaceID != nil {
		spaceExists, err = mysqlSpaceExists(ctx, tx, *spaceID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read note space")
		}
	}
	if spaceID != nil && spaceExists && !actor.Admin {
		actorMember, err = mysqlSpaceMemberActive(ctx, tx, *spaceID, delete.ActorUserID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read note membership")
		}
	}
	actorCanRead := store.NoteDeleteActorCanRead(rowStatus, visibility, spaceID, spaceExists, actorMember || actor.Admin)

	deleteIDs, err := listMySQLCommentSubtreeNoteIDs(ctx, tx, delete.NoteID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect comment notes")
	}
	attachments, err := deleteMySQLNoteSetTx(ctx, tx, deleteIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete note set")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit note delete transaction")
	}
	return &store.DeleteNoteWithPolicyResult{ActorCanRead: actorCanRead, Attachments: attachments}, nil
}

// listMySQLCommentSubtreeNoteIDs returns the target note plus every note in
// its comment subtree (comments, replies to comments, and so on), following
// COMMENT relations from parent to child.
func listMySQLCommentSubtreeNoteIDs(ctx context.Context, tx *sql.Tx, rootNoteID int32) ([]int32, error) {
	ids := []int32{rootNoteID}
	seen := map[int32]struct{}{rootNoteID: {}}
	frontier := []int32{rootNoteID}
	for len(frontier) > 0 {
		var next []int32
		for _, batch := range deleteUserBatches(frontier, deleteUserBatchSize) {
			clause, args := deleteUserInClause(1, batch)
			if err := func() error {
				rows, err := tx.QueryContext(ctx, "SELECT note_id FROM note_relation WHERE type = 'COMMENT' AND related_note_id IN "+clause, args...)
				if err != nil {
					return errors.Wrap(err, "failed to list comment notes")
				}
				defer rows.Close()
				for rows.Next() {
					var childID int32
					if err := rows.Scan(&childID); err != nil {
						return errors.Wrap(err, "failed to read comment note id")
					}
					if _, ok := seen[childID]; ok {
						continue
					}
					seen[childID] = struct{}{}
					next = append(next, childID)
				}
				return errors.Wrap(rows.Err(), "failed to list comment notes")
			}(); err != nil {
				return nil, err
			}
		}
		// Comment creation takes the same note locks before adding replies.
		locked, err := lockMySQLCommentNoteIDs(ctx, tx, next)
		if err != nil {
			return nil, err
		}
		ids = append(ids, locked...)
		frontier = locked
	}
	return ids, nil
}

func lockMySQLCommentNoteIDs(ctx context.Context, tx *sql.Tx, ids []int32) ([]int32, error) {
	locked := make([]int32, 0, len(ids))
	for _, batch := range deleteUserBatches(ids, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if err := func() error {
			rows, err := tx.QueryContext(ctx, "SELECT id FROM note WHERE id IN "+clause+" ORDER BY id FOR UPDATE", args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id int32
				if err := rows.Scan(&id); err != nil {
					return err
				}
				locked = append(locked, id)
			}
			return rows.Err()
		}(); err != nil {
			return nil, errors.Wrap(err, "failed to lock comment notes")
		}
	}
	return locked, nil
}

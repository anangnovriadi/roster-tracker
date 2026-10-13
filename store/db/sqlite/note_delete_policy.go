package sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) DeleteNoteWithPolicy(ctx context.Context, delete *store.DeleteNoteWithPolicy) (*store.DeleteNoteWithPolicyResult, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin note delete transaction")
	}
	defer func() { _ = tx.Rollback() }()

	actor, err := requireSQLiteActiveNoteActor(ctx, tx, delete.ActorUserID)
	if err != nil {
		return nil, err
	}
	var creatorID int32
	var rowStatus store.RowStatus
	var visibility store.Visibility
	var space sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT creator_id, row_status, visibility, space_id FROM note WHERE id = ?", delete.NoteID).Scan(
		&creatorID, &rowStatus, &visibility, &space,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNoteMutationConflict
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to read note")
	} else if creatorID != delete.ActorUserID && !actor.Admin {
		return nil, store.ErrNotePermissionDenied
	}
	spaceID := store.NullInt32Pointer(space)
	spaceExists := false
	actorMember := false
	if spaceID != nil {
		var existingSpaceID int32
		if err := tx.QueryRowContext(ctx, "SELECT id FROM space WHERE id = ?", *spaceID).Scan(&existingSpaceID); err == nil {
			spaceExists = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, errors.Wrap(err, "failed to read note space")
		}
		if spaceExists && !actor.Admin {
			actorMember, err = sqliteSpaceMemberActive(ctx, tx, *spaceID, delete.ActorUserID)
			if err != nil {
				return nil, errors.Wrap(err, "failed to read note membership")
			}
		}
	}
	actorCanRead := store.NoteDeleteActorCanRead(rowStatus, visibility, spaceID, spaceExists, actorMember || actor.Admin)

	deleteIDs, err := listSQLiteCommentSubtreeNoteIDs(ctx, tx, delete.NoteID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect comment notes")
	}
	attachments, err := deleteSQLiteNoteSetTx(ctx, tx, deleteIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete note set")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit note delete transaction")
	}
	return &store.DeleteNoteWithPolicyResult{ActorCanRead: actorCanRead, Attachments: attachments}, nil
}

// listSQLiteCommentSubtreeNoteIDs returns the target note plus every note in
// its comment subtree (comments, replies to comments, and so on), following
// COMMENT relations from parent to child.
func listSQLiteCommentSubtreeNoteIDs(ctx context.Context, tx dbExecutor, rootNoteID int32) ([]int32, error) {
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
					ids = append(ids, childID)
					next = append(next, childID)
				}
				return errors.Wrap(rows.Err(), "failed to list comment notes")
			}(); err != nil {
				return nil, err
			}
		}
		frontier = next
	}
	return ids, nil
}

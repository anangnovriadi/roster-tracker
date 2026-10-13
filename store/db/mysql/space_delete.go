package mysql

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

// DeleteSpace hard-deletes only notes directly placed in the Space. Relations
// are removed when either endpoint is deleted, but are never traversed.
func (d *DB) DeleteSpace(ctx context.Context, delete *store.DeleteSpace) (*store.DeleteSpaceResult, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin space delete transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if err := requireMySQLSpaceDeleteAdmin(ctx, tx, delete); err != nil {
		return nil, err
	}
	noteIDs, err := listMySQLSpaceDeleteNoteIDs(ctx, tx, delete.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect assigned notes")
	}
	attachments, err := deleteMySQLNoteSetTx(ctx, tx, noteIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete assigned note set")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM space_member WHERE space_id = ?", delete.ID); err != nil {
		return nil, errors.Wrap(err, "failed to delete space memberships")
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM space WHERE id = ?", delete.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete space")
	}
	if count, err := result.RowsAffected(); err != nil {
		return nil, errors.Wrap(err, "failed to count deleted spaces")
	} else if count != 1 {
		return nil, store.ErrSpaceNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit space deletion")
	}
	return &store.DeleteSpaceResult{Attachments: attachments}, nil
}

func requireMySQLSpaceDeleteAdmin(ctx context.Context, tx *sql.Tx, delete *store.DeleteSpace) error {
	statuses, err := readMySQLUserStatuses(ctx, tx, delete.ActorUserID)
	if err != nil {
		return err
	}
	if statuses[delete.ActorUserID] != store.Normal {
		return store.ErrSpacePermissionDenied
	}
	var spaceID int32
	// Invitation creation takes the same lock before inserting its relationship.
	if err := tx.QueryRowContext(ctx, "SELECT id FROM space WHERE id = ? FOR UPDATE", delete.ID).Scan(&spaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrSpaceNotFound
		}
		return err
	}
	var role store.SpaceMemberRole
	if err := tx.QueryRowContext(ctx, "SELECT role FROM space_member WHERE space_id = ? AND user_id = ? AND status = 'ACTIVE'", delete.ID, delete.ActorUserID).Scan(&role); errors.Is(err, sql.ErrNoRows) {
		return store.ErrSpacePermissionDenied
	} else if err != nil {
		return errors.Wrap(err, "failed to read space administrator membership")
	} else if role != store.SpaceMemberRoleAdmin {
		return store.ErrSpacePermissionDenied
	}
	return nil
}

func listMySQLSpaceDeleteNoteIDs(ctx context.Context, tx *sql.Tx, spaceID int32) ([]int32, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM note WHERE space_id = ? ORDER BY id", spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int32, 0)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func deleteMySQLNoteSetTx(ctx context.Context, tx *sql.Tx, noteIDs []int32) ([]*store.Attachment, error) {
	attachments, err := listMySQLNoteSetAttachments(ctx, tx, noteIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect note attachments")
	}
	if err := deleteMySQLNoteSetShares(ctx, tx, noteIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete note shares")
	}
	if err := deleteNotesTx(ctx, tx, noteIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete notes")
	}
	if err := deleteReactionsByNoteIDsTx(ctx, tx, noteIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete note reactions")
	}
	if err := deleteAttachmentsByIDsTx(ctx, tx, attachmentIDsFromList(attachments)); err != nil {
		return nil, errors.Wrap(err, "failed to delete note attachments")
	}
	if err := deleteNoteRelationsTx(ctx, tx, noteIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete incident note relations")
	}
	return attachments, nil
}

func listMySQLNoteSetAttachments(ctx context.Context, tx *sql.Tx, noteIDs []int32) ([]*store.Attachment, error) {
	attachments := make([]*store.Attachment, 0)
	seen := make(map[int32]struct{})
	for _, batch := range deleteUserBatches(noteIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if err := appendDeleteUserAttachments(ctx, tx, `SELECT id, uid, creator_id, note_id, storage_type, reference, payload
			FROM attachment WHERE note_id IN `+clause+` ORDER BY id`, args, seen, &attachments); err != nil {
			return nil, err
		}
	}
	return attachments, nil
}

func deleteMySQLNoteSetShares(ctx context.Context, tx *sql.Tx, noteIDs []int32) error {
	for _, batch := range deleteUserBatches(noteIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if _, err := tx.ExecContext(ctx, "DELETE FROM note_share WHERE note_id IN "+clause, args...); err != nil {
			return err
		}
	}
	return nil
}

package postgres

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) DeleteAttachmentsWithPolicy(ctx context.Context, policy *store.AttachmentDeletionPolicy, attachmentIDs []int32) error {
	if policy == nil || policy.ActorUserID <= 0 {
		return store.ErrNotePermissionDenied
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin attachment delete transaction")
	}
	defer func() { _ = tx.Rollback() }()

	actor, err := requirePostgresActiveNoteActor(ctx, tx, policy.ActorUserID)
	if err != nil {
		return err
	}
	attachments, err := listPostgresAttachmentSnapshots(ctx, tx, attachmentIDs)
	if err != nil {
		return errors.Wrap(err, "failed to read attachment delete targets")
	}
	noteIDs, err := store.ValidateAttachmentMutationTargets(policy.ActorUserID, actor.Admin, attachmentIDs, attachments)
	if err != nil {
		return err
	}
	if err := store.ValidateAttachmentDeletionNoteSnapshots(noteIDs, policy.ExpectedNoteContents); err != nil {
		return err
	}
	if err := authorizePostgresAttachmentMutation(ctx, tx, policy.ActorUserID, actor.Admin, noteIDs, policy.ExpectedNoteContents); err != nil {
		return err
	}

	for _, attachmentID := range attachmentIDs {
		result, err := tx.ExecContext(ctx, "DELETE FROM attachment WHERE id = $1", attachmentID)
		if err != nil {
			return errors.Wrap(err, "failed to delete attachment")
		}
		if rows, err := result.RowsAffected(); err != nil {
			return errors.Wrap(err, "failed to count deleted attachment")
		} else if rows != 1 {
			return store.ErrNoteMutationConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit attachment deletion")
	}
	return nil
}

func listPostgresAttachmentSnapshots(ctx context.Context, tx *sql.Tx, attachmentIDs []int32) ([]*store.Attachment, error) {
	attachments := make([]*store.Attachment, 0, len(attachmentIDs))
	seen := make(map[int32]struct{}, len(attachmentIDs))
	for _, batch := range deleteUserBatches(attachmentIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if err := appendDeleteUserAttachments(ctx, tx, `SELECT id, uid, creator_id, note_id, storage_type, reference, payload
			FROM attachment WHERE id IN `+clause+` ORDER BY id`, args, seen, &attachments); err != nil {
			return nil, err
		}
	}
	return attachments, nil
}

func authorizePostgresAttachmentMutation(
	ctx context.Context,
	tx *sql.Tx,
	actorUserID int32,
	actorIsAdmin bool,
	noteIDs []int32,
	expectedNoteContents map[int32]string,
) error {
	writePolicy := &store.NoteWritePolicy{ActorUserID: actorUserID}
	for _, noteID := range noteIDs {
		snapshot := &store.NoteWriteSnapshot{ActorIsAdmin: actorIsAdmin}
		var noteSpace sql.NullInt64
		var content string
		if err := tx.QueryRowContext(ctx, `SELECT creator_id, row_status, space_id, visibility, content
			FROM note WHERE id = $1`, noteID).Scan(
			&snapshot.CreatorID, &snapshot.RowStatus, &noteSpace, &snapshot.Visibility, &content,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return store.ErrNoteMutationConflict
			}
			return errors.Wrap(err, "failed to read attachment note")
		}
		snapshot.SpaceID = store.NullInt32Pointer(noteSpace)
		if snapshot.SpaceID != nil {
			var err error
			snapshot.SourceSpaceExists, snapshot.SourceMemberActive, err = readPostgresNoteSpaceState(ctx, tx, *snapshot.SpaceID, actorUserID)
			if err != nil {
				return errors.Wrap(err, "failed to read attachment note space state")
			}
		}
		if err := store.ValidateNoteWriteSnapshot(writePolicy, nil, snapshot); err != nil {
			return err
		}
		if expectedNoteContents != nil {
			expectedContent, ok := expectedNoteContents[noteID]
			if !ok || content != expectedContent {
				return store.ErrNoteMutationConflict
			}
		}
	}
	return nil
}

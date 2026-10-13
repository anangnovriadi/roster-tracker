package mysql

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/anangnovriadi/roster-tracker/store"
)

// ApplyNoteMutation atomically updates a note, attachment bindings, and reference relations.
func (d *DB) ApplyNoteMutation(ctx context.Context, mutation *store.NoteMutation) error {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return errors.Wrap(err, "failed to begin note transaction")
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if mutation.CommentContextNoteID != nil {
		// Deletion locks this row before scanning its replies. Lock it before
		// any serializable snapshot reads in this transaction.
		var contextNoteID int32
		if err := tx.QueryRowContext(ctx, "SELECT id FROM note WHERE id = ? FOR UPDATE", *mutation.CommentContextNoteID).Scan(&contextNoteID); errors.Is(err, sql.ErrNoRows) {
			return store.ErrNoteMutationConflict
		} else if err != nil {
			return errors.Wrap(err, "failed to lock comment context note")
		}
	}
	if err := validateMySQLNoteRelationEndpoints(ctx, tx, mutation); err != nil {
		return err
	}
	if create := mutation.NoteCreate; create != nil {
		if err := validateMySQLNoteCreate(ctx, tx, create); err != nil {
			return err
		}
		if mutation.CommentContextNoteID != nil {
			if err := authorizeMySQLNoteComment(ctx, tx, *mutation.CommentContextNoteID, create.CreatorID); err != nil {
				return err
			}
		}
		created, err := insertMySQLNote(ctx, tx, create)
		if err != nil {
			return err
		}
		*create = *created
		mutation.NoteID = create.ID
		mutation.NoteCreatorID = create.CreatorID
		mutation.ExpectedNoteContent = create.Content
		for _, relation := range mutation.ReferenceRelations {
			if relation != nil {
				relation.NoteID = create.ID
			}
		}
		if mutation.CommentContextNoteID != nil {
			if err := insertMySQLNoteCommentRelation(ctx, tx, create.ID, *mutation.CommentContextNoteID); err != nil {
				return err
			}
		}
	}
	policy := mutation.WritePolicy()
	actorUserID := mutation.ActorUserID()
	actorIsAdmin := false
	if policy != nil {
		snapshot, err := validateMySQLNoteWritePolicy(ctx, tx, mutation.NoteID, policy, mutation.NoteUpdate)
		if err != nil {
			return err
		}
		actorIsAdmin = snapshot.ActorIsAdmin
	}

	var creatorID int32
	var content string
	if err := tx.QueryRowContext(ctx, "SELECT `creator_id`, `content` FROM `note` WHERE `id` = ? FOR UPDATE", mutation.NoteID).Scan(&creatorID, &content); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Wrap(store.ErrNoteMutationConflict, "note no longer exists")
		}
		return errors.Wrap(err, "failed to lock note")
	}
	if creatorID != mutation.NoteCreatorID || content != mutation.ExpectedNoteContent {
		return errors.Wrap(store.ErrNoteMutationConflict, "note changed while applying mutation")
	}
	removedAttachments, err := listMySQLAttachmentsByIDs(ctx, tx, mutation.RemovedAttachmentIDs)
	if err != nil {
		return errors.Wrap(err, "failed to read removed attachments")
	}
	if len(removedAttachments) != len(mutation.RemovedAttachmentIDs) {
		return errors.Wrap(store.ErrNoteMutationConflict, "removed attachment no longer exists")
	}
	for _, attachment := range removedAttachments {
		if !store.NoteAttachmentRemovalAllowed(attachment, mutation.NoteID, actorUserID, actorIsAdmin) {
			return errors.Wrap(store.ErrNoteMutationConflict, "attachment is no longer removable from the note")
		}
	}

	for _, binding := range mutation.Bindings {
		var attachmentCreatorID int32
		var noteID sql.NullInt32
		if err := tx.QueryRowContext(ctx, "SELECT `creator_id`, `note_id` FROM `attachment` WHERE `id` = ? FOR UPDATE", binding.ID).Scan(&attachmentCreatorID, &noteID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Wrapf(store.ErrNoteMutationConflict, "attachment %s no longer exists", binding.UID)
			}
			return errors.Wrap(err, "failed to lock attachment")
		}
		if binding.WasBoundToNote {
			if !noteID.Valid || noteID.Int32 != mutation.NoteID {
				return errors.Wrapf(store.ErrNoteMutationConflict, "attachment %s is no longer bound to the note", binding.UID)
			}
		} else if !store.NoteAttachmentBindingOwnerAllowed(attachmentCreatorID, mutation.NoteCreatorID, actorUserID, actorIsAdmin) || noteID.Valid {
			return errors.Wrapf(store.ErrNoteMutationConflict, "attachment %s is no longer available", binding.UID)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE `attachment` SET `note_id` = ?, `updated_ts` = FROM_UNIXTIME(?) WHERE `id` = ?", mutation.NoteID, binding.UpdatedTs, binding.ID); err != nil {
			return errors.Wrap(err, "failed to bind attachment")
		}
	}
	for _, attachment := range removedAttachments {
		result, err := tx.ExecContext(ctx, "DELETE FROM `attachment` WHERE `id` = ? AND `note_id` = ?", attachment.ID, mutation.NoteID)
		if err != nil {
			return errors.Wrap(err, "failed to delete removed attachment")
		}
		if rows, err := result.RowsAffected(); err != nil {
			return errors.Wrap(err, "failed to count deleted removed attachment")
		} else if rows != 1 {
			return errors.Wrap(store.ErrNoteMutationConflict, "attachment is no longer bound to the note")
		}
	}

	for _, attachmentID := range mutation.RequiredAttachmentIDs {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM `attachment` WHERE `id` = ? AND `note_id` = ? FOR UPDATE", attachmentID, mutation.NoteID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(store.ErrNoteMutationConflict, "a referenced attachment is no longer bound to the note")
			}
			return errors.Wrap(err, "failed to verify referenced attachment")
		}
	}

	if mutation.NoteUpdate != nil {
		if mutation.NoteUpdate.ID != mutation.NoteID {
			return errors.New("note update target does not match attachment mutation")
		}
		if err := applyNoteUpdate(ctx, tx, mutation.NoteUpdate); err != nil {
			return err
		}
	}
	if mutation.ReplaceReferenceRelations {
		if err := replaceNoteReferenceRelations(ctx, tx, mutation.NoteID, mutation.ReferenceRelations); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit note transaction")
	}
	return nil
}

func insertMySQLNoteCommentRelation(ctx context.Context, tx *sql.Tx, noteID, contextNoteID int32) error {
	if noteID <= 0 || contextNoteID <= 0 || noteID == contextNoteID {
		return errors.New("invalid COMMENT relation")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO note_relation (note_id, related_note_id, type) VALUES (?, ?, ?)",
		noteID, contextNoteID, store.NoteRelationComment); err != nil {
		return errors.Wrap(err, "failed to insert COMMENT relation")
	}
	return nil
}

func replaceNoteReferenceRelations(ctx context.Context, tx *sql.Tx, noteID int32, relations []*store.NoteRelation) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM `note_relation` WHERE `note_id` = ? AND `type` = ?", noteID, store.NoteRelationReference); err != nil {
		return errors.Wrap(err, "failed to delete note reference relations")
	}
	for _, relation := range relations {
		if relation == nil || relation.NoteID != noteID || relation.Type != store.NoteRelationReference {
			return errors.New("invalid note reference relation mutation")
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO `note_relation` (`note_id`, `related_note_id`, `type`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `type` = `type`",
			relation.NoteID, relation.RelatedNoteID, relation.Type,
		); err != nil {
			return errors.Wrap(err, "failed to insert note reference relation")
		}
	}
	return nil
}

type noteUpdateExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func applyNoteUpdate(ctx context.Context, executor noteUpdateExecer, update *store.UpdateNote) error {
	set, args := []string{}, []any{}
	if v := update.UID; v != nil {
		set, args = append(set, "`uid` = ?"), append(args, *v)
	}
	if v := update.CreatedTs; v != nil {
		set, args = append(set, "`created_ts` = FROM_UNIXTIME(?)"), append(args, *v)
	}
	if v := update.UpdatedTs; v != nil {
		set, args = append(set, "`updated_ts` = FROM_UNIXTIME(?)"), append(args, *v)
	}
	if v := update.RowStatus; v != nil {
		set, args = append(set, "`row_status` = ?"), append(args, *v)
	}
	if v := update.Content; v != nil {
		set, args = append(set, "`content` = ?"), append(args, *v)
	}
	if v := update.Visibility; v != nil {
		set, args = append(set, "`visibility` = ?"), append(args, *v)
	}
	if v := update.Pinned; v != nil {
		set, args = append(set, "`pinned` = ?"), append(args, *v)
	}
	if v := update.Payload; v != nil {
		payload, err := protojson.Marshal(v)
		if err != nil {
			return errors.Wrap(err, "failed to marshal note payload")
		}
		set, args = append(set, "`payload` = ?"), append(args, string(payload))
	}
	if update.ClearSpace {
		set = append(set, "`space_id` = NULL")
	} else if v := update.SpaceID; v != nil {
		set, args = append(set, "`space_id` = ?"), append(args, *v)
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, update.ID)
	if _, err := executor.ExecContext(ctx, "UPDATE `note` SET "+strings.Join(set, ", ")+" WHERE `id` = ?", args...); err != nil {
		return errors.Wrap(err, "failed to update note")
	}
	return nil
}

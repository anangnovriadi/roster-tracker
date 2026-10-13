package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/anangnovriadi/roster-tracker/filter"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) CreateAttachment(ctx context.Context, create *store.Attachment) (*store.Attachment, error) {
	if create.Policy == nil {
		return insertPostgresAttachment(ctx, d.db, create)
	}
	if create.NoteID == nil || create.CreatorID != create.Policy.ActorUserID {
		return nil, store.ErrNotePermissionDenied
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := validatePostgresNoteWritePolicy(ctx, tx, *create.NoteID, create.Policy, nil); err != nil {
		return nil, err
	}
	attachment, err := insertPostgresAttachment(ctx, tx, create)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return attachment, nil
}

type postgresAttachmentQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertPostgresAttachment(ctx context.Context, executor postgresAttachmentQueryRower, create *store.Attachment) (*store.Attachment, error) {
	fields := []string{"uid", "filename", "blob", "type", "size", "creator_id", "note_id", "storage_type", "reference", "payload"}
	storageType := ""
	if create.StorageType != storepb.AttachmentStorageType_ATTACHMENT_STORAGE_TYPE_UNSPECIFIED {
		storageType = create.StorageType.String()
	}
	payloadString := "{}"
	if create.Payload != nil {
		bytes, err := protojson.Marshal(create.Payload)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal attachment payload")
		}
		payloadString = string(bytes)
	}
	args := []any{create.UID, create.Filename, create.Blob, create.Type, create.Size, create.CreatorID, create.NoteID, storageType, create.Reference, payloadString}

	stmt := "INSERT INTO attachment (" + strings.Join(fields, ", ") + ") VALUES (" + placeholders(len(args)) + ") RETURNING id, created_ts, updated_ts"
	if err := executor.QueryRowContext(ctx, stmt, args...).Scan(&create.ID, &create.CreatedTs, &create.UpdatedTs); err != nil {
		return nil, err
	}
	return create, nil
}

func (d *DB) ListAttachments(ctx context.Context, find *store.FindAttachment) ([]*store.Attachment, error) {
	where, args := []string{"1 = 1"}, []any{}

	if v := find.ID; v != nil {
		where, args = append(where, "attachment.id = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := find.UID; v != nil {
		where, args = append(where, "attachment.uid = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := find.CreatorID; v != nil {
		where, args = append(where, "attachment.creator_id = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := find.Filename; v != nil {
		where, args = append(where, "attachment.filename = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := find.FilenameSearch; v != nil {
		where, args = append(where, "attachment.filename LIKE "+placeholder(len(args)+1)), append(args, fmt.Sprintf("%%%s%%", *v))
	}
	if v := find.NoteID; v != nil {
		where, args = append(where, "attachment.note_id = "+placeholder(len(args)+1)), append(args, *v)
	}
	if len(find.NoteIDList) > 0 {
		holders := make([]string, 0, len(find.NoteIDList))
		for _, id := range find.NoteIDList {
			holders = append(holders, placeholder(len(args)+1))
			args = append(args, id)
		}
		where = append(where, "attachment.note_id IN ("+strings.Join(holders, ", ")+")")
	}
	if find.HasRelatedNote {
		where = append(where, "attachment.note_id IS NOT NULL")
	}
	if len(find.Filters) > 0 {
		engine, err := filter.DefaultAttachmentEngine()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get filter engine")
		}
		if err := filter.AppendConditions(ctx, engine, find.Filters, filter.DialectPostgres, &where, &args); err != nil {
			return nil, errors.Wrap(err, "failed to append filter conditions")
		}
	}
	if access := find.Access; access != nil {
		scopeClauses := []string{}
		if access.UserID != nil {
			activeHolder := placeholder(len(args) + 1)
			args = append(args, *access.UserID)
			creatorHolder := placeholder(len(args) + 1)
			args = append(args, *access.UserID)
			scopeClauses = append(scopeClauses, `(attachment.note_id IS NULL AND EXISTS (SELECT 1 FROM "user" AS attachment_user WHERE attachment_user.id = `+activeHolder+` AND attachment_user.row_status = 'NORMAL') AND attachment.creator_id = `+creatorHolder+`)`)
		}
		scopeClauses = append(scopeClauses, "(attachment.note_id IS NOT NULL AND "+postgresNoteAccessPredicate(access, "note", "attachment_member", &args)+")")
		if len(scopeClauses) == 0 {
			where = append(where, "1 = 0")
		} else {
			where = append(where, "("+strings.Join(scopeClauses, " OR ")+")")
		}
	}

	fields := []string{
		"attachment.id AS id",
		"attachment.uid AS uid",
		"attachment.filename AS filename",
		"attachment.type AS type",
		"attachment.size AS size",
		"attachment.creator_id AS creator_id",
		"attachment.created_ts AS created_ts",
		"attachment.updated_ts AS updated_ts",
		"attachment.note_id AS note_id",
		"attachment.storage_type AS storage_type",
		"attachment.reference AS reference",
		"attachment.payload AS payload",
		"CASE WHEN note.uid IS NOT NULL THEN note.uid ELSE NULL END AS note_uid",
		"attachment_creator.username AS creator_username",
	}
	if find.GetBlob {
		fields = append(fields, "attachment.blob AS blob")
	}

	query := fmt.Sprintf(`
		SELECT
			%s
		FROM attachment
		LEFT JOIN note ON attachment.note_id = note.id
		LEFT JOIN "user" AS attachment_creator ON attachment.creator_id = attachment_creator.id
		LEFT JOIN space AS attachment_space ON note.space_id = attachment_space.id
		WHERE %s
		ORDER BY attachment.updated_ts DESC
	`, strings.Join(fields, ", "), strings.Join(where, " AND "))
	if find.Limit != nil {
		query = fmt.Sprintf("%s LIMIT %d", query, *find.Limit)
		if find.Offset != nil {
			query = fmt.Sprintf("%s OFFSET %d", query, *find.Offset)
		}
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*store.Attachment, 0)
	for rows.Next() {
		attachment := store.Attachment{}
		var noteID sql.NullInt32
		var storageType string
		var payloadBytes []byte
		dests := []any{
			&attachment.ID,
			&attachment.UID,
			&attachment.Filename,
			&attachment.Type,
			&attachment.Size,
			&attachment.CreatorID,
			&attachment.CreatedTs,
			&attachment.UpdatedTs,
			&noteID,
			&storageType,
			&attachment.Reference,
			&payloadBytes,
			&attachment.NoteUID,
			&attachment.CreatorUsername,
		}
		if find.GetBlob {
			dests = append(dests, &attachment.Blob)
		}
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}

		if noteID.Valid {
			attachment.NoteID = &noteID.Int32
		}
		attachment.StorageType = storepb.AttachmentStorageType(storepb.AttachmentStorageType_value[storageType])
		payload := &storepb.AttachmentPayload{}
		if err := protojsonUnmarshaler.Unmarshal(payloadBytes, payload); err != nil {
			return nil, err
		}
		attachment.Payload = payload
		list = append(list, &attachment)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (d *DB) UpdateAttachment(ctx context.Context, update *store.UpdateAttachment) error {
	if update.Policy == nil {
		return applyPostgresAttachmentUpdate(ctx, d.db, update)
	}
	if update.NoteID != nil {
		return store.ErrNoteMutationConflict
	}
	attachmentIDs := []int32{update.ID}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	actor, err := requirePostgresActiveNoteActor(ctx, tx, update.Policy.ActorUserID)
	if err != nil {
		return err
	}
	attachments, err := listPostgresAttachmentSnapshots(ctx, tx, attachmentIDs)
	if err != nil {
		return err
	}
	noteIDs, err := store.ValidateAttachmentMutationTargets(update.Policy.ActorUserID, actor.Admin, attachmentIDs, attachments)
	if err != nil {
		return err
	}
	if err := authorizePostgresAttachmentMutation(ctx, tx, update.Policy.ActorUserID, actor.Admin, noteIDs, nil); err != nil {
		return err
	}
	if err := applyPostgresAttachmentUpdate(ctx, tx, update); err != nil {
		return err
	}
	return tx.Commit()
}

func applyPostgresAttachmentUpdate(ctx context.Context, executor noteUpdateExecer, update *store.UpdateAttachment) error {
	set, args := []string{}, []any{}

	if v := update.UID; v != nil {
		set, args = append(set, "uid = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := update.UpdatedTs; v != nil {
		set, args = append(set, "updated_ts = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := update.Filename; v != nil {
		set, args = append(set, "filename = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := update.NoteID; v != nil {
		set, args = append(set, "note_id = "+placeholder(len(args)+1)), append(args, *v)
	}
	if v := update.Payload; v != nil {
		bytes, err := protojson.Marshal(v)
		if err != nil {
			return errors.Wrap(err, "failed to marshal attachment payload")
		}
		set, args = append(set, "payload = "+placeholder(len(args)+1)), append(args, string(bytes))
	}

	if len(set) == 0 {
		return nil
	}
	stmt := `UPDATE attachment SET ` + strings.Join(set, ", ") + ` WHERE id = ` + placeholder(len(args)+1)
	args = append(args, update.ID)
	result, err := executor.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	if _, err := result.RowsAffected(); err != nil {
		return err
	}
	return nil
}

func (d *DB) DeleteAttachment(ctx context.Context, delete *store.DeleteAttachment) error {
	return d.DeleteAttachments(ctx, []*store.DeleteAttachment{delete})
}

func (d *DB) DeleteAttachments(ctx context.Context, deletes []*store.DeleteAttachment) error {
	if len(deletes) == 0 {
		return nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to start attachment delete transaction")
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	stmt := `DELETE FROM attachment WHERE id = $1`
	for _, delete := range deletes {
		result, err := tx.ExecContext(ctx, stmt, delete.ID)
		if err != nil {
			return err
		}
		if _, err := result.RowsAffected(); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

// GetAttachmentStorageUsage returns the total persisted attachment size for a creator in bytes.
func (d *DB) GetAttachmentStorageUsage(ctx context.Context, creatorID int32) (int64, error) {
	var size int64
	if err := d.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(size), 0) FROM attachment WHERE creator_id = $1 AND storage_type <> 'EXTERNAL'", creatorID).Scan(&size); err != nil {
		return 0, errors.Wrap(err, "failed to sum attachment sizes")
	}
	return size, nil
}

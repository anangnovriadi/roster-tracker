package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/anangnovriadi/roster-tracker/filter"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) CreateNote(ctx context.Context, create *store.Note) (*store.Note, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateSQLiteNoteCreate(ctx, tx, create); err != nil {
		return nil, err
	}
	if err := insertSQLiteNote(ctx, tx, create); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return create, nil
}

func insertSQLiteNote(ctx context.Context, tx dbExecutor, create *store.Note) error {
	fields := []string{"`uid`", "`creator_id`", "`content`", "`visibility`", "`payload`", "`space_id`"}
	placeholder := []string{"?", "?", "?", "?", "?", "?"}
	payload := "{}"
	if create.Payload != nil {
		payloadBytes, err := protojson.Marshal(create.Payload)
		if err != nil {
			return err
		}
		payload = string(payloadBytes)
	}
	args := []any{create.UID, create.CreatorID, create.Content, create.Visibility, payload, create.SpaceID}

	// Add custom timestamps if provided
	if create.CreatedTs != 0 {
		fields = append(fields, "`created_ts`")
		placeholder = append(placeholder, "?")
		args = append(args, create.CreatedTs)
	}
	if create.UpdatedTs != 0 {
		fields = append(fields, "`updated_ts`")
		placeholder = append(placeholder, "?")
		args = append(args, create.UpdatedTs)
	}

	stmt := "INSERT INTO `note` (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(placeholder, ", ") + ") RETURNING `id`, `created_ts`, `updated_ts`, `row_status`"
	if err := tx.QueryRowContext(ctx, stmt, args...).Scan(
		&create.ID,
		&create.CreatedTs,
		&create.UpdatedTs,
		&create.RowStatus,
	); err != nil {
		return err
	}
	return nil
}

func validateSQLiteNoteCreate(ctx context.Context, tx dbExecutor, create *store.Note) error {
	actor, err := readSQLiteNoteActor(ctx, tx, create.CreatorID)
	if err != nil {
		return err
	}
	if !actor.Active {
		return store.ErrNoteSpaceMembershipRequired
	}
	if create.SpaceID == nil {
		return nil
	}
	if !actor.Admin {
		return validateSQLiteNoteSpaceMember(ctx, tx, *create.SpaceID, create.CreatorID)
	}
	// An instance administrator places notes without membership; the target
	// Space must still exist.
	exists, err := sqliteSpaceExists(ctx, tx, *create.SpaceID)
	if err != nil {
		return err
	}
	if !exists {
		return store.ErrNoteSpaceNotWritable
	}
	return nil
}

func validateSQLiteNoteSpaceMember(ctx context.Context, tx dbExecutor, spaceID, userID int32) error {
	exists, err := sqliteSpaceExists(ctx, tx, spaceID)
	if err != nil {
		return err
	}
	if !exists {
		return store.ErrNoteSpaceNotWritable
	}
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM space_member sm JOIN user u ON u.id = sm.user_id
		WHERE sm.space_id = ? AND sm.user_id = ? AND sm.status = 'ACTIVE'
			AND sm.role IN ('ADMIN', 'USER') AND u.row_status = 'NORMAL')`, spaceID, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return store.ErrNoteSpaceMembershipRequired
	}
	return nil
}

func (d *DB) ListNotes(ctx context.Context, find *store.FindNote) ([]*store.Note, error) {
	where, args := []string{"1 = 1"}, []any{}

	engine, err := filter.DefaultEngine()
	if err != nil {
		return nil, err
	}
	if err := filter.AppendConditions(ctx, engine, find.Filters, filter.DialectSQLite, &where, &args); err != nil {
		return nil, err
	}
	if v := find.ID; v != nil {
		where, args = append(where, "`note`.`id` = ?"), append(args, *v)
	}
	if len(find.IDList) > 0 {
		placeholders := make([]string, 0, len(find.IDList))
		for range find.IDList {
			placeholders = append(placeholders, "?")
		}
		where = append(where, "`note`.`id` IN ("+strings.Join(placeholders, ",")+")")
		for _, id := range find.IDList {
			args = append(args, id)
		}
	}
	if v := find.UID; v != nil {
		where, args = append(where, "`note`.`uid` = ?"), append(args, *v)
	}
	if len(find.UIDList) > 0 {
		placeholders := make([]string, 0, len(find.UIDList))
		for range find.UIDList {
			placeholders = append(placeholders, "?")
		}
		where = append(where, "`note`.`uid` IN ("+strings.Join(placeholders, ",")+")")
		for _, uid := range find.UIDList {
			args = append(args, uid)
		}
	}
	if v := find.CreatorID; v != nil {
		where, args = append(where, "`note`.`creator_id` = ?"), append(args, *v)
	}
	if v := find.RowStatus; v != nil {
		where, args = append(where, "`note`.`row_status` = ?"), append(args, *v)
	}
	if v := find.VisibilityList; len(v) != 0 {
		placeholder := []string{}
		for _, visibility := range v {
			placeholder = append(placeholder, "?")
			args = append(args, visibility.String())
		}
		where = append(where, fmt.Sprintf("`note`.`visibility` IN (%s)", strings.Join(placeholder, ",")))
	}
	if v := find.CommentContextNoteID; v != nil {
		where, args = append(where, `EXISTS (
			SELECT 1 FROM note_relation AS comment_context
			WHERE comment_context.note_id = note.id
				AND comment_context.related_note_id = ?
				AND comment_context.type = 'COMMENT'
		)`), append(args, *v)
	}
	if access := find.Access; access != nil {
		where = append(where, sqliteNoteAccessPredicate(access, "`note`", "`access_member`", &args))
	}
	if find.ExcludeComments {
		where = append(where, `NOT EXISTS (
			SELECT 1 FROM note_relation AS comment_relation
			WHERE comment_relation.note_id = note.id AND comment_relation.type = 'COMMENT'
		)`)
	}

	order := "DESC"
	if find.OrderByTimeAsc {
		order = "ASC"
	}
	orderBy := []string{}
	if find.OrderByPinned {
		orderBy = append(orderBy, "`pinned` DESC")
	}
	if find.OrderByUpdatedTs {
		orderBy = append(orderBy, "`updated_ts` "+order)
	} else {
		orderBy = append(orderBy, "`created_ts` "+order)
	}
	// Add id as final tie-breaker
	orderBy = append(orderBy, "`id` DESC")
	fields := []string{
		"`note`.`id` AS `id`",
		"`note`.`uid` AS `uid`",
		"`note`.`creator_id` AS `creator_id`",
		"`note`.`created_ts` AS `created_ts`",
		"`note`.`updated_ts` AS `updated_ts`",
		"`note`.`row_status` AS `row_status`",
		"`note`.`visibility` AS `visibility`",
		"`note`.`pinned` AS `pinned`",
		"`note`.`payload` AS `payload`",
		"`note`.`space_id` AS `space_id`",
		`(SELECT parent_note.uid
			FROM note_relation AS parent_relation
			JOIN note AS parent_note ON parent_note.id = parent_relation.related_note_id
			WHERE parent_relation.note_id = note.id AND parent_relation.type = 'COMMENT'
			ORDER BY parent_note.id LIMIT 1) AS parent_uid`,
	}
	if !find.ExcludeContent {
		fields = append(fields, "`note`.`content` AS `content`")
	}

	query := "SELECT " + strings.Join(fields, ", ") + " FROM `note` " +
		"LEFT JOIN `user` AS `note_creator` ON `note`.`creator_id` = `note_creator`.`id` " +
		"LEFT JOIN `space` AS `note_space` ON `note`.`space_id` = `note_space`.`id` " +
		"WHERE " + strings.Join(where, " AND ") + " " +
		"ORDER BY " + strings.Join(orderBy, ", ")
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

	list := make([]*store.Note, 0)
	for rows.Next() {
		var note store.Note
		var payloadBytes []byte
		dests := []any{
			&note.ID,
			&note.UID,
			&note.CreatorID,
			&note.CreatedTs,
			&note.UpdatedTs,
			&note.RowStatus,
			&note.Visibility,
			&note.Pinned,
			&payloadBytes,
			&note.SpaceID,
			&note.ParentUID,
		}
		if !find.ExcludeContent {
			dests = append(dests, &note.Content)
		}
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}
		payload := &storepb.NotePayload{}
		if err := protojsonUnmarshaler.Unmarshal(payloadBytes, payload); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal payload")
		}
		note.Payload = payload
		list = append(list, &note)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

// CountUserNotes counts the notes a user created, comments included, in every
// row status and Space.
func (d *DB) CountUserNotes(ctx context.Context, creatorID int32) (int64, error) {
	stmt := `SELECT COUNT(*) FROM note WHERE creator_id = ?`
	var count int64
	if err := d.db.QueryRowContext(ctx, stmt, creatorID).Scan(&count); err != nil {
		return 0, errors.Wrap(err, "failed to count user notes")
	}
	return count, nil
}

func (d *DB) UpdateNote(ctx context.Context, update *store.UpdateNote) error {
	if update.Policy == nil {
		return applyNoteUpdate(ctx, d.db, update)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := validateSQLiteNoteWritePolicy(ctx, tx, update.ID, update.Policy, update); err != nil {
		return err
	}
	if err := applyNoteUpdate(ctx, tx, update); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) DeleteNote(ctx context.Context, delete *store.DeleteNote) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to start note delete transaction")
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM `note` WHERE `id` = ?", delete.ID); err != nil {
		return errors.Wrap(err, "failed to delete note")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM `reaction` WHERE `note_id` = ?", delete.ID); err != nil {
		return errors.Wrap(err, "failed to delete note reactions")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit note delete transaction")
	}
	return nil
}

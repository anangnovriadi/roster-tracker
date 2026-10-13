package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/anangnovriadi/roster-tracker/filter"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) UpsertNoteRelation(ctx context.Context, create *store.NoteRelation) (*store.NoteRelation, error) {
	stmt := "INSERT INTO `note_relation` (`note_id`, `related_note_id`, `type`) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE `type` = `type`"
	_, err := d.db.ExecContext(
		ctx,
		stmt,
		create.NoteID,
		create.RelatedNoteID,
		create.Type,
	)
	if err != nil {
		return nil, err
	}

	noteRelation := store.NoteRelation{
		NoteID:        create.NoteID,
		RelatedNoteID: create.RelatedNoteID,
		Type:          create.Type,
	}

	return &noteRelation, nil
}

func (d *DB) ListNoteRelations(ctx context.Context, find *store.FindNoteRelation) ([]*store.NoteRelation, error) {
	where, args := []string{"TRUE"}, []any{}
	if find.NoteID != nil {
		where, args = append(where, "`note_id` = ?"), append(args, *find.NoteID)
	}
	if find.RelatedNoteID != nil {
		where, args = append(where, "`related_note_id` = ?"), append(args, *find.RelatedNoteID)
	}
	if find.Type != nil {
		where, args = append(where, "`type` = ?"), append(args, *find.Type)
	}
	if len(find.NoteIDList) > 0 {
		placeholders := make([]string, len(find.NoteIDList))
		for i, id := range find.NoteIDList {
			placeholders[i] = "?"
			args = append(args, id)
		}
		inClause := strings.Join(placeholders, ", ")
		for _, id := range find.NoteIDList {
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("(`note_id` IN (%s) OR `related_note_id` IN (%s))", inClause, inClause))
	}
	if len(find.SourceNoteIDList) > 0 {
		placeholders := make([]string, len(find.SourceNoteIDList))
		for i, id := range find.SourceNoteIDList {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("`note_id` IN (%s)", strings.Join(placeholders, ", ")))
	}
	if len(find.RelatedNoteIDList) > 0 {
		placeholders := make([]string, len(find.RelatedNoteIDList))
		for i, id := range find.RelatedNoteIDList {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, fmt.Sprintf("`related_note_id` IN (%s)", strings.Join(placeholders, ", ")))
	}
	if find.SourceNoteRowStatus != nil {
		where = append(where, "`note_id` IN (SELECT `id` FROM `note` WHERE `row_status` = ?)")
		args = append(args, *find.SourceNoteRowStatus)
	}
	if find.NoteFilter != nil {
		engine, err := filter.DefaultEngine()
		if err != nil {
			return nil, err
		}
		stmt, err := engine.CompileToStatement(ctx, *find.NoteFilter, filter.RenderOptions{
			Dialect:           filter.DialectMySQL,
			PlaceholderOffset: 0,
		})
		if err != nil {
			return nil, err
		}
		if stmt.SQL != "" {
			where = append(where, fmt.Sprintf("note_id IN (SELECT id FROM note WHERE %s)", stmt.SQL))
			where = append(where, fmt.Sprintf("related_note_id IN (SELECT id FROM note WHERE %s)", stmt.SQL))
			args = append(args, append(stmt.Args, stmt.Args...)...)
		}
	}

	query := "SELECT `note_id`, `related_note_id`, `type` FROM `note_relation` WHERE " + strings.Join(where, " AND ") + " ORDER BY `note_id` DESC"
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

	list := []*store.NoteRelation{}
	for rows.Next() {
		noteRelation := &store.NoteRelation{}
		if err := rows.Scan(
			&noteRelation.NoteID,
			&noteRelation.RelatedNoteID,
			&noteRelation.Type,
		); err != nil {
			return nil, err
		}
		list = append(list, noteRelation)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (d *DB) DeleteNoteRelation(ctx context.Context, delete *store.DeleteNoteRelation) error {
	if err := store.ValidateNoteRelationDelete(delete); err != nil {
		return err
	}
	where, args := []string{"TRUE"}, []any{}
	if delete.NoteID != nil {
		where, args = append(where, "`note_id` = ?"), append(args, *delete.NoteID)
	}
	if delete.RelatedNoteID != nil {
		where, args = append(where, "`related_note_id` = ?"), append(args, *delete.RelatedNoteID)
	}
	where, args = append(where, "`type` = ?"), append(args, *delete.Type)
	stmt := "DELETE FROM `note_relation` WHERE " + strings.Join(where, " AND ")
	result, err := d.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	if _, err = result.RowsAffected(); err != nil {
		return err
	}
	return nil
}

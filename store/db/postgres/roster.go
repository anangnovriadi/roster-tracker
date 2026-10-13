package postgres

import (
	"context"
	"strings"

	"github.com/usememos/memos/store"
)

func (d *DB) CreateRosterAssignment(ctx context.Context, create *store.RosterAssignment) (*store.RosterAssignment, error) {
	if err := d.db.QueryRowContext(ctx, `
		INSERT INTO roster_assignment (duty_date, user_id)
		VALUES ($1, $2)
		RETURNING id, created_ts
	`, create.DutyDate, create.UserID).Scan(&create.ID, &create.CreatedTs); err != nil {
		return nil, err
	}
	return create, nil
}

func (d *DB) ListRosterAssignments(ctx context.Context, find *store.FindRosterAssignment) ([]*store.RosterAssignment, error) {
	where, args := []string{"1 = 1"}, []any{}
	if find.ID != nil {
		where, args = append(where, "id = "+placeholder(len(args)+1)), append(args, *find.ID)
	}
	if find.UserID != nil {
		where, args = append(where, "user_id = "+placeholder(len(args)+1)), append(args, *find.UserID)
	}
	if find.DutyDate != nil {
		where, args = append(where, "duty_date = "+placeholder(len(args)+1)), append(args, *find.DutyDate)
	}
	if find.StartDate != nil {
		where, args = append(where, "duty_date >= "+placeholder(len(args)+1)), append(args, *find.StartDate)
	}
	if find.EndDate != nil {
		where, args = append(where, "duty_date <= "+placeholder(len(args)+1)), append(args, *find.EndDate)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT id, created_ts, duty_date, user_id
		FROM roster_assignment
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY duty_date ASC, id ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*store.RosterAssignment{}
	for rows.Next() {
		assignment := &store.RosterAssignment{}
		if err := rows.Scan(&assignment.ID, &assignment.CreatedTs, &assignment.DutyDate, &assignment.UserID); err != nil {
			return nil, err
		}
		list = append(list, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (d *DB) DeleteRosterAssignment(ctx context.Context, delete *store.DeleteRosterAssignment) error {
	_, err := d.db.ExecContext(ctx, "DELETE FROM roster_assignment WHERE id = $1", delete.ID)
	return err
}

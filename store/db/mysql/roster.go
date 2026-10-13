package mysql

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func (d *DB) CreateRosterAssignment(ctx context.Context, create *store.RosterAssignment) (*store.RosterAssignment, error) {
	result, err := d.db.ExecContext(ctx, "INSERT INTO `roster_assignment` (`duty_date`, `user_id`) VALUES (?, ?)", create.DutyDate, create.UserID)
	if err != nil {
		return nil, err
	}
	rawID, err := result.LastInsertId()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read roster assignment id")
	}
	id := int32(rawID)
	list, err := d.ListRosterAssignments(ctx, &store.FindRosterAssignment{ID: &id})
	if err != nil {
		return nil, err
	}
	if len(list) != 1 {
		return nil, errors.Errorf("unexpected roster assignment count %d after insert", len(list))
	}
	return list[0], nil
}

func (d *DB) ListRosterAssignments(ctx context.Context, find *store.FindRosterAssignment) ([]*store.RosterAssignment, error) {
	where, args := []string{"1 = 1"}, []any{}
	if find.ID != nil {
		where, args = append(where, "`id` = ?"), append(args, *find.ID)
	}
	if find.UserID != nil {
		where, args = append(where, "`user_id` = ?"), append(args, *find.UserID)
	}
	if find.DutyDate != nil {
		where, args = append(where, "`duty_date` = ?"), append(args, *find.DutyDate)
	}
	if find.StartDate != nil {
		where, args = append(where, "`duty_date` >= ?"), append(args, *find.StartDate)
	}
	if find.EndDate != nil {
		where, args = append(where, "`duty_date` <= ?"), append(args, *find.EndDate)
	}

	rows, err := d.db.QueryContext(ctx, "SELECT `id`, UNIX_TIMESTAMP(`created_ts`), `duty_date`, `user_id` FROM `roster_assignment` WHERE "+
		strings.Join(where, " AND ")+" ORDER BY `duty_date` ASC, `id` ASC", args...)
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
	_, err := d.db.ExecContext(ctx, "DELETE FROM `roster_assignment` WHERE `id` = ?", delete.ID)
	return err
}

func (d *DB) DeleteUserRosterAssignments(ctx context.Context, delete *store.DeleteUserRosterAssignments) error {
	if delete.FromDate == "" {
		_, err := d.db.ExecContext(ctx, "DELETE FROM roster_assignment WHERE user_id = ?", delete.UserID)
		return err
	}
	_, err := d.db.ExecContext(ctx, "DELETE FROM roster_assignment WHERE user_id = ? AND duty_date >= ?", delete.UserID, delete.FromDate)
	return err
}

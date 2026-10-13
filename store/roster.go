package store

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/pkg/errors"
)

// RosterDateLayout is the format of a roster duty date.
const RosterDateLayout = "2006-01-02"

// ErrRosterAssignmentExists indicates that someone is already on duty that day.
var ErrRosterAssignmentExists = stderrors.New("roster assignment already exists")

// RosterAssignment puts one member on duty for one day.
type RosterAssignment struct {
	ID        int32
	CreatedTs int64
	// DutyDate is the day on duty, formatted with RosterDateLayout.
	DutyDate string
	UserID   int32
}

// FindRosterAssignment filters roster assignments. Dates are inclusive.
type FindRosterAssignment struct {
	ID        *int32
	UserID    *int32
	DutyDate  *string
	StartDate *string
	EndDate   *string
}

// DeleteRosterAssignment deletes one roster assignment.
type DeleteRosterAssignment struct {
	ID int32
}

// ValidateRosterDate reports whether value is a calendar day in RosterDateLayout.
func ValidateRosterDate(value string) error {
	parsed, err := time.Parse(RosterDateLayout, value)
	if err != nil || parsed.Format(RosterDateLayout) != value {
		return errors.Errorf("invalid roster date %q, expected YYYY-MM-DD", value)
	}
	return nil
}

func (s *Store) CreateRosterAssignment(ctx context.Context, create *RosterAssignment) (*RosterAssignment, error) {
	if err := ValidateRosterDate(create.DutyDate); err != nil {
		return nil, err
	}
	// One member is on duty per day.
	existing, err := s.driver.ListRosterAssignments(ctx, &FindRosterAssignment{DutyDate: &create.DutyDate})
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return nil, ErrRosterAssignmentExists
	}
	return s.driver.CreateRosterAssignment(ctx, create)
}

func (s *Store) ListRosterAssignments(ctx context.Context, find *FindRosterAssignment) ([]*RosterAssignment, error) {
	return s.driver.ListRosterAssignments(ctx, find)
}

func (s *Store) GetRosterAssignment(ctx context.Context, find *FindRosterAssignment) (*RosterAssignment, error) {
	list, err := s.driver.ListRosterAssignments(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

func (s *Store) DeleteRosterAssignment(ctx context.Context, delete *DeleteRosterAssignment) error {
	return s.driver.DeleteRosterAssignment(ctx, delete)
}

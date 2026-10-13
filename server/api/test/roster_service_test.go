package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestRosterAssignments(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	admin, err := ts.CreateHostUser(ctx, "admin")
	require.NoError(t, err)
	alice, err := ts.CreateRegularUser(ctx, "alice")
	require.NoError(t, err)
	bob, err := ts.CreateRegularUser(ctx, "bob")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)
	aliceCtx := ts.CreateUserContext(ctx, alice.ID)

	create := func(ctx context.Context, date, username string) (*apipb.RosterAssignment, error) {
		return ts.Service.CreateRosterAssignment(ctx, &apipb.CreateRosterAssignmentRequest{
			RosterAssignment: &apipb.RosterAssignment{Date: date, User: "users/" + username},
		})
	}

	_, err = create(aliceCtx, "2026-10-12", "alice")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = create(ctx, "2026-10-12", "alice")
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = create(adminCtx, "2026-13-01", "alice")
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	// Admins run the roster and cannot be put on duty themselves.
	_, err = create(adminCtx, "2026-10-12", "admin")
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	first, err := create(adminCtx, "2026-10-12", "alice")
	require.NoError(t, err)
	require.Equal(t, "2026-10-12", first.Date)
	require.Equal(t, "users/alice", first.User)
	// One member is on duty per day.
	_, err = create(adminCtx, "2026-10-12", "bob")
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	_, err = create(adminCtx, "2026-10-12", "alice")
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	_, err = create(adminCtx, "2026-10-20", "bob")
	require.NoError(t, err)

	// Signed-out visitors can read the roster.
	list, err := ts.Service.ListRosterAssignments(ctx, &apipb.ListRosterAssignmentsRequest{StartDate: "2026-10-12", EndDate: "2026-10-12"})
	require.NoError(t, err)
	require.Len(t, list.RosterAssignments, 1)
	require.Equal(t, "users/alice", list.RosterAssignments[0].User)

	_, err = ts.Service.ListRosterAssignments(ctx, &apipb.ListRosterAssignmentsRequest{StartDate: "2026-10-12", EndDate: "2026-10-01"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	// Archived members drop off the roster.
	archived := store.Archived
	_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{ID: bob.ID, RowStatus: &archived})
	require.NoError(t, err)
	list, err = ts.Service.ListRosterAssignments(ctx, &apipb.ListRosterAssignmentsRequest{StartDate: "2026-10-01", EndDate: "2026-10-31"})
	require.NoError(t, err)
	require.Len(t, list.RosterAssignments, 1)

	_, err = ts.Service.DeleteRosterAssignment(aliceCtx, &apipb.DeleteRosterAssignmentRequest{Name: first.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = ts.Service.DeleteRosterAssignment(adminCtx, &apipb.DeleteRosterAssignmentRequest{Name: first.Name})
	require.NoError(t, err)
	_, err = ts.Service.DeleteRosterAssignment(adminCtx, &apipb.DeleteRosterAssignmentRequest{Name: first.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestArchivingAndDeletingMemberClearsRoster(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	admin, err := ts.CreateHostUser(ctx, "admin")
	require.NoError(t, err)
	alice, err := ts.CreateRegularUser(ctx, "alice")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)

	today := time.Now()
	past := today.AddDate(0, 0, -3).Format(store.RosterDateLayout)
	todayDate := today.Format(store.RosterDateLayout)
	future := today.AddDate(0, 0, 3).Format(store.RosterDateLayout)
	for _, date := range []string{past, todayDate, future} {
		_, err := ts.Service.CreateRosterAssignment(adminCtx, &apipb.CreateRosterAssignmentRequest{
			RosterAssignment: &apipb.RosterAssignment{Date: date, User: "users/alice"},
		})
		require.NoError(t, err)
	}
	dutyDates := func() []string {
		t.Helper()
		list, err := ts.Store.ListRosterAssignments(ctx, &store.FindRosterAssignment{UserID: &alice.ID})
		require.NoError(t, err)
		dates := []string{}
		for _, assignment := range list {
			dates = append(dates, assignment.DutyDate)
		}
		return dates
	}

	// Archiving takes the member off every roster day, past ones included.
	_, err = ts.Service.UpdateUser(adminCtx, &apipb.UpdateUserRequest{
		User:       &apipb.User{Name: "users/alice", State: apipb.State_ARCHIVED},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	require.NoError(t, err)
	require.Empty(t, dutyDates())

	// Restoring does not put the member back on the roster; they must be assigned again.
	_, err = ts.Service.UpdateUser(adminCtx, &apipb.UpdateUserRequest{
		User:       &apipb.User{Name: "users/alice", State: apipb.State_NORMAL},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	require.NoError(t, err)
	require.Empty(t, dutyDates())
	_, err = ts.Service.CreateRosterAssignment(adminCtx, &apipb.CreateRosterAssignmentRequest{
		RosterAssignment: &apipb.RosterAssignment{Date: future, User: "users/alice"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{future}, dutyDates())

	// Deleting the member removes every remaining roster day.
	_, err = ts.Service.DeleteUser(adminCtx, &apipb.DeleteUserRequest{Name: "users/alice"})
	require.NoError(t, err)
	require.Empty(t, dutyDates())
}

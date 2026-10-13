package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
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

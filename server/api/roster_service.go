package api

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

// maxRosterListDays bounds one ListRosterAssignments request.
const maxRosterListDays = 366

func (s *APIService) ListRosterAssignments(ctx context.Context, request *apipb.ListRosterAssignmentsRequest) (*apipb.ListRosterAssignmentsResponse, error) {
	if err := store.ValidateRosterDate(request.StartDate); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start_date: %v", err)
	}
	if err := store.ValidateRosterDate(request.EndDate); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid end_date: %v", err)
	}
	start, _ := time.Parse(store.RosterDateLayout, request.StartDate)
	end, _ := time.Parse(store.RosterDateLayout, request.EndDate)
	if end.Before(start) {
		return nil, status.Errorf(codes.InvalidArgument, "end_date must not be before start_date")
	}
	if end.Sub(start) > maxRosterListDays*24*time.Hour {
		return nil, status.Errorf(codes.InvalidArgument, "the date range must be at most %d days", maxRosterListDays)
	}

	assignments, err := s.Store.ListRosterAssignments(ctx, &store.FindRosterAssignment{
		StartDate: &request.StartDate,
		EndDate:   &request.EndDate,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list roster assignments: %v", err)
	}
	if len(assignments) == 0 {
		return &apipb.ListRosterAssignmentsResponse{}, nil
	}

	userIDs := make([]int32, 0, len(assignments))
	for _, assignment := range assignments {
		userIDs = append(userIDs, assignment.UserID)
	}
	normal := store.Normal
	users, err := s.Store.ListUsers(ctx, &store.FindUser{IDList: userIDs, RowStatus: &normal})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}
	usersByID := make(map[int32]*store.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	response := &apipb.ListRosterAssignmentsResponse{}
	for _, assignment := range assignments {
		// Deleted and archived members drop off the roster.
		user, ok := usersByID[assignment.UserID]
		if !ok {
			continue
		}
		response.RosterAssignments = append(response.RosterAssignments, convertRosterAssignmentFromStore(assignment, user))
	}
	return response, nil
}

func (s *APIService) CreateRosterAssignment(ctx context.Context, request *apipb.CreateRosterAssignmentRequest) (*apipb.RosterAssignment, error) {
	if _, err := s.requireRosterAdmin(ctx); err != nil {
		return nil, err
	}
	if request.RosterAssignment == nil {
		return nil, status.Errorf(codes.InvalidArgument, "roster_assignment is required")
	}
	if err := store.ValidateRosterDate(request.RosterAssignment.Date); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	user, err := ResolveUserByName(ctx, s.Store, request.RosterAssignment.User)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user: %v", err)
	}
	if user == nil || user.RowStatus != store.Normal {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	// Admins run the roster; only members go on duty.
	if user.Role == store.RoleAdmin {
		return nil, status.Errorf(codes.InvalidArgument, "admins cannot be put on duty")
	}

	assignment, err := s.Store.CreateRosterAssignment(ctx, &store.RosterAssignment{
		DutyDate: request.RosterAssignment.Date,
		UserID:   user.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrRosterAssignmentExists) {
			return nil, status.Errorf(codes.AlreadyExists, "someone is already on duty that day")
		}
		return nil, status.Errorf(codes.Internal, "failed to create roster assignment: %v", err)
	}
	return convertRosterAssignmentFromStore(assignment, user), nil
}

func (s *APIService) DeleteRosterAssignment(ctx context.Context, request *apipb.DeleteRosterAssignmentRequest) (*emptypb.Empty, error) {
	if _, err := s.requireRosterAdmin(ctx); err != nil {
		return nil, err
	}
	id, err := ExtractRosterAssignmentIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid roster assignment name: %v", err)
	}
	assignment, err := s.Store.GetRosterAssignment(ctx, &store.FindRosterAssignment{ID: &id})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get roster assignment: %v", err)
	}
	if assignment == nil {
		return nil, status.Errorf(codes.NotFound, "roster assignment not found")
	}
	if err := s.Store.DeleteRosterAssignment(ctx, &store.DeleteRosterAssignment{ID: id}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete roster assignment: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *APIService) requireRosterAdmin(ctx context.Context) (*store.User, error) {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	return currentUser, nil
}

func convertRosterAssignmentFromStore(assignment *store.RosterAssignment, user *store.User) *apipb.RosterAssignment {
	return &apipb.RosterAssignment{
		Name:       fmt.Sprintf("%s%d", RosterAssignmentNamePrefix, assignment.ID),
		Date:       assignment.DutyDate,
		User:       BuildUserName(user.Username),
		CreateTime: timestamppb.New(time.Unix(assignment.CreatedTs, 0)),
	}
}

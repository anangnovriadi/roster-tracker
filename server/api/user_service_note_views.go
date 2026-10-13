package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/anangnovriadi/roster-tracker/filter"
	"github.com/anangnovriadi/roster-tracker/internal/random"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/server/auth"
	"github.com/anangnovriadi/roster-tracker/store"
)

// Helper function to extract user and note view ID from a note view resource name.
// Format: users/{user}/views/{view}.
func (s *APIService) extractUserAndNoteViewIDFromName(ctx context.Context, name string) (*store.User, string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "users" || parts[2] != "views" {
		return nil, "", errors.Errorf("invalid note view name format: %s", name)
	}

	user, err := ResolveUserByName(ctx, s.Store, BuildUserName(parts[1]))
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		return nil, "", errors.Errorf("user not found: %s", parts[1])
	}

	noteViewID := parts[3]
	if noteViewID == "" {
		return nil, "", errors.Errorf("empty note view ID in name: %s", name)
	}

	return user, noteViewID, nil
}

// Helper function to construct a note view resource name.
func constructNoteViewName(username string, noteViewID string) string {
	return fmt.Sprintf("%s/views/%s", BuildUserName(username), noteViewID)
}

func convertNoteViewFromStore(username string, noteView *storepb.NoteViewsUserSetting_NoteView) *apipb.NoteView {
	return &apipb.NoteView{
		Name:   constructNoteViewName(username, noteView.GetId()),
		Title:  noteView.GetTitle(),
		Filter: noteView.GetFilter(),
		Icon:   convertNoteViewIconFromStore(noteView.GetIcon()),
	}
}

// requireCallerIs asserts that the caller is user. Personal resources such as
// note views and note exports are never reachable by another account.
func (s *APIService) requireCallerIs(ctx context.Context, user *store.User) error {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if currentUser == nil {
		return status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.ID != user.ID {
		return status.Errorf(codes.PermissionDenied, "permission denied")
	}
	return nil
}

// ListNoteViews lists the saved note views owned by a user.
func (s *APIService) ListNoteViews(ctx context.Context, request *apipb.ListNoteViewsRequest) (*apipb.ListNoteViewsResponse, error) {
	user, err := ResolveUserByName(ctx, s.Store, request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}

	storeNoteViews, err := s.Store.GetUserNoteViews(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note views: %v", err)
	}

	noteViews := []*apipb.NoteView{}
	for _, noteView := range storeNoteViews {
		noteViews = append(noteViews, convertNoteViewFromStore(user.Username, noteView))
	}

	return &apipb.ListNoteViewsResponse{
		NoteViews: noteViews,
	}, nil
}

// GetNoteView returns a saved note view by resource name.
func (s *APIService) GetNoteView(ctx context.Context, request *apipb.GetNoteViewRequest) (*apipb.NoteView, error) {
	user, noteViewID, err := s.extractUserAndNoteViewIDFromName(ctx, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note view name: %v", err)
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}

	noteViews, err := s.Store.GetUserNoteViews(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note views: %v", err)
	}
	for _, noteView := range noteViews {
		if noteView.GetId() == noteViewID {
			return convertNoteViewFromStore(user.Username, noteView), nil
		}
	}

	return nil, status.Errorf(codes.NotFound, "note view not found")
}

// CreateNoteView creates a saved note view for a user.
func (s *APIService) CreateNoteView(ctx context.Context, request *apipb.CreateNoteViewRequest) (*apipb.NoteView, error) {
	user, err := ResolveUserByName(ctx, s.Store, request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(auth.GetUserID(ctx)), 1); err != nil {
		return nil, err
	}

	newNoteView := &storepb.NoteViewsUserSetting_NoteView{
		Id:     random.UUID(),
		Title:  request.GetNoteView().GetTitle(),
		Filter: request.GetNoteView().GetFilter(),
		Icon:   convertNoteViewIconToStore(request.GetNoteView().GetIcon()),
	}
	if newNoteView.Title == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}
	if err := s.validateFilter(ctx, newNoteView.Filter); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
	}
	if err := store.ValidateNoteViewIcon(newNoteView.Icon); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid icon: %v", err)
	}
	if request.ValidateOnly {
		return convertNoteViewFromStore(user.Username, newNoteView), nil
	}

	if err := s.Store.AddUserNoteView(ctx, user.ID, newNoteView); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create note view: %v", err)
	}

	return convertNoteViewFromStore(user.Username, newNoteView), nil
}

// UpdateNoteView updates the selected fields of a saved note view.
func (s *APIService) UpdateNoteView(ctx context.Context, request *apipb.UpdateNoteViewRequest) (*apipb.NoteView, error) {
	user, noteViewID, err := s.extractUserAndNoteViewIDFromName(ctx, request.GetNoteView().GetName())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note view name: %v", err)
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}

	var title, filterValue *string
	var icon **storepb.NoteViewsUserSetting_NoteView_Icon
	for _, field := range request.UpdateMask.Paths {
		switch field {
		case "title":
			if request.GetNoteView().GetTitle() == "" {
				return nil, status.Errorf(codes.InvalidArgument, "title is required")
			}
			value := request.GetNoteView().GetTitle()
			title = &value
		case "filter":
			if err := s.validateFilter(ctx, request.GetNoteView().GetFilter()); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
			}
			value := request.GetNoteView().GetFilter()
			filterValue = &value
		case "icon":
			value := convertNoteViewIconToStore(request.GetNoteView().GetIcon())
			if err := store.ValidateNoteViewIcon(value); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid icon: %v", err)
			}
			icon = &value
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported update mask path: %s", field)
		}
	}

	updatedNoteView, err := s.Store.UpdateUserNoteView(ctx, user.ID, noteViewID, title, filterValue, icon)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update note view: %v", err)
	}
	if updatedNoteView == nil {
		return nil, status.Errorf(codes.NotFound, "note view not found")
	}

	return convertNoteViewFromStore(user.Username, updatedNoteView), nil
}

// DeleteNoteView deletes a saved note view by resource name.
func (s *APIService) DeleteNoteView(ctx context.Context, request *apipb.DeleteNoteViewRequest) (*emptypb.Empty, error) {
	user, noteViewID, err := s.extractUserAndNoteViewIDFromName(ctx, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note view name: %v", err)
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}

	found, err := s.Store.RemoveUserNoteView(ctx, user.ID, noteViewID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete note view: %v", err)
	}
	if !found {
		return nil, status.Errorf(codes.NotFound, "note view not found")
	}

	return &emptypb.Empty{}, nil
}

func (s *APIService) validateFilter(ctx context.Context, filterStr string) error {
	if filterStr == "" {
		return errors.New("filter cannot be empty")
	}

	engine, err := filter.DefaultEngine()
	if err != nil {
		return err
	}

	if _, err := engine.CompileToStatement(ctx, filterStr, filter.RenderOptions{Dialect: s.filterDialect()}); err != nil {
		return errors.Wrap(err, "failed to compile filter")
	}
	return nil
}

// filterDialect maps the configured database driver onto the filter engine dialect.
func (s *APIService) filterDialect() filter.DialectName {
	switch s.Profile.Driver {
	case "mysql":
		return filter.DialectMySQL
	case "postgres":
		return filter.DialectPostgres
	default:
		return filter.DialectSQLite
	}
}

package api

import (
	"context"
	stderrors "errors"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/store"
)

// buildNoteReadContext resolves authorization inputs for exactly one note.
// Relations never contribute access to either endpoint.
func (s *APIService) buildNoteReadContext(ctx context.Context, note *store.Note, sharedNoteID *int32) (access.NoteReadContext, error) {
	viewer, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return access.NoteReadContext{}, status.Errorf(codes.Internal, "failed to get user")
	}
	allowAnonymous := false
	if viewer == nil {
		allowAnonymous, err = s.Store.AllowsAnonymousAccess(ctx)
		if err != nil {
			return access.NoteReadContext{}, status.Errorf(codes.Internal, "failed to resolve instance access policy")
		}
	}
	return s.buildNoteReadContextForViewer(ctx, note, viewer, allowAnonymous, sharedNoteID)
}

func (s *APIService) buildNoteReadContextForViewer(ctx context.Context, note *store.Note, viewer *store.User, allowAnonymous bool, sharedNoteID *int32) (access.NoteReadContext, error) {
	if note == nil {
		return access.NoteReadContext{}, status.Error(codes.NotFound, "note not found")
	}
	readContext, err := access.ResolveNoteReadContext(ctx, s.Store, note, viewer, allowAnonymous, sharedNoteID)
	if err != nil {
		return access.NoteReadContext{}, status.Errorf(codes.Internal, "failed to resolve note access")
	}
	return readContext, nil
}

func (s *APIService) checkNoteReadAccess(ctx context.Context, note *store.Note) error {
	readContext, err := s.buildNoteReadContext(ctx, note, nil)
	if err != nil {
		return err
	}
	return noteAccessDecisionError(access.CheckNoteReadContext(readContext))
}

func noteAccessDecisionError(decision access.NoteReadDecision) error {
	switch decision.Denial {
	case access.NoteReadDenialNone:
		return nil
	case access.NoteReadDenialNotFound:
		return status.Error(codes.NotFound, "note not found")
	case access.NoteReadDenialUnauthenticated:
		return status.Error(codes.Unauthenticated, "user not authenticated")
	default:
		return status.Error(codes.PermissionDenied, "permission denied")
	}
}

// newNoteAccessScope returns the note-local authorization predicate for a
// caller. Drivers apply it as a database predicate before LIMIT/OFFSET so
// inaccessible rows can neither leak nor skew counts and pagination.
func newNoteAccessScope(currentUser *store.User, allowPublic bool) *store.NoteAccessScope {
	accessScope := &store.NoteAccessScope{AllowPublic: allowPublic, AllowProtected: currentUser != nil}
	if currentUser != nil {
		accessScope.UserID = &currentUser.ID
	}
	return accessScope
}

// resolveNoteAccessScope resolves the caller and builds their note access
// scope. For an anonymous caller the instance access policy decides whether
// PUBLIC notes are readable at all. Callers map the returned error to their own
// transport representation.
func (s *APIService) resolveNoteAccessScope(ctx context.Context) (*store.NoteAccessScope, *store.User, error) {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get current user")
	}
	allowPublic := currentUser != nil
	if currentUser == nil {
		allowPublic, err = s.Store.AllowsAnonymousAccess(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to resolve instance access policy")
		}
	}
	return newNoteAccessScope(currentUser, allowPublic), currentUser, nil
}

// resolveSpaceByName resolves a space resource name to an existing Space
// without any membership check.
func (s *APIService) resolveSpaceByName(ctx context.Context, name string) (*store.Space, error) {
	spaceUID, err := ExtractSpaceUIDFromName(name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space name: %v", err)
	}
	space, err := s.Store.GetSpace(ctx, &store.FindSpace{UID: &spaceUID})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get space")
	}
	if space == nil {
		return nil, status.Error(codes.NotFound, "space not found")
	}
	return space, nil
}

// resolveWritableSpaceByName resolves a space resource name and requires the
// caller to be an active member of it. A non-member receives NotFound so that
// an existing collaboration boundary stays indistinguishable from a missing
// resource.
func (s *APIService) resolveWritableSpaceByName(ctx context.Context, name string, userID int32) (*store.Space, error) {
	space, err := s.resolveSpaceByName(ctx, name)
	if err != nil {
		return nil, err
	}
	active, err := s.isActiveSpaceMember(ctx, space.ID, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to resolve space membership")
	}
	if !active {
		return nil, status.Error(codes.NotFound, "space not found")
	}
	return space, nil
}

// resolveSpaceForNotePlacement resolves the Space a note is being placed in.
// An instance administrator may place notes in any existing Space; every
// other caller must be an active member. Collection scopes such as the space
// filter keep using resolveWritableSpaceByName, so feeds stay membership-only.
func (s *APIService) resolveSpaceForNotePlacement(ctx context.Context, name string, user *store.User) (*store.Space, error) {
	if access.IsInstanceAdmin(user) {
		return s.resolveSpaceByName(ctx, name)
	}
	return s.resolveWritableSpaceByName(ctx, name, user.ID)
}

func (s *APIService) isActiveSpaceMember(ctx context.Context, spaceID, userID int32) (bool, error) {
	user, err := s.Store.GetUser(ctx, &store.FindUser{ID: &userID})
	if err != nil {
		return false, err
	}
	if user == nil || user.RowStatus != store.Normal {
		return false, nil
	}
	membership, err := s.Store.GetSpaceMember(ctx, &store.FindSpaceMember{SpaceID: &spaceID, UserID: &userID})
	if err != nil {
		return false, err
	}
	return membership != nil && membership.Role.IsActiveMember(), nil
}

func sameOptionalInt32(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func noteWritePolicy(actorUserID int32, lifecycleOnly bool) *store.NoteWritePolicy {
	return &store.NoteWritePolicy{
		ActorUserID:   actorUserID,
		LifecycleOnly: lifecycleOnly,
	}
}

func mapNoteWriteError(err error, operation string) error {
	switch {
	case stderrors.Is(err, store.ErrNoteMutationConflict):
		return status.Errorf(codes.FailedPrecondition, "note state changed: %v", err)
	case stderrors.Is(err, store.ErrNoteSpaceNotWritable):
		return status.Error(codes.FailedPrecondition, "note space is no longer writable")
	case stderrors.Is(err, store.ErrNoteSpaceMembershipRequired), stderrors.Is(err, store.ErrNotePermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case stderrors.Is(err, store.ErrNoteShareConflict):
		return status.Error(codes.FailedPrecondition, "revoke active shares before using the SPACE audience")
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}

// requireAssignedNoteWritable requires the placement of an assigned note to be
// valid and the actor to be an active member of it. An instance administrator
// is exempt from membership but not from placement validity.
func (s *APIService) requireAssignedNoteWritable(ctx context.Context, note *store.Note, user *store.User) error {
	if note.SpaceID == nil {
		return nil
	}
	space, err := s.Store.GetSpace(ctx, &store.FindSpace{ID: note.SpaceID})
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get note space")
	}
	if space == nil {
		return status.Errorf(codes.FailedPrecondition, "note has invalid space placement")
	}
	if access.IsInstanceAdmin(user) {
		return nil
	}
	active, err := s.isActiveSpaceMember(ctx, space.ID, user.ID)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to resolve space membership")
	}
	if !active {
		return status.Errorf(codes.PermissionDenied, "active space membership is required")
	}
	return nil
}

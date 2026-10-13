package api

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/lithammer/shortuuid/v4"
	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

// CreateNoteShare creates an opaque share link for a note.
// Only the note's creator may call this.
func (s *APIService) CreateNoteShare(ctx context.Context, request *apipb.CreateNoteShareRequest) (*apipb.NoteShare, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}

	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}
	if note.RowStatus != store.Normal {
		return nil, status.Errorf(codes.FailedPrecondition, "only active notes can be shared")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	if note.Visibility == store.SpaceAudience {
		return nil, status.Errorf(codes.FailedPrecondition, "SPACE audience notes cannot be shared")
	}
	var expiresTs *int64
	if request.NoteShare != nil && request.NoteShare.ExpireTime != nil {
		ts := request.NoteShare.ExpireTime.AsTime().Unix()
		if ts <= time.Now().Unix() {
			return nil, status.Errorf(codes.InvalidArgument, "expire_time must be in the future")
		}
		expiresTs = &ts
	}

	// Generate a URL-safe token using shortuuid (base57-encoded UUID v4, 22 chars, 122-bit entropy).
	policy := noteWritePolicy(user.ID, false)
	policy.CreatingShare = true
	ms, err := s.Store.CreateNoteShare(ctx, &store.NoteShare{
		UID:       shortuuid.New(),
		NoteID:    note.ID,
		CreatorID: user.ID,
		ExpiresTs: expiresTs,
		Policy:    policy,
	})
	if err != nil {
		return nil, mapNoteWriteError(err, "failed to create note share")
	}

	return convertNoteShareFromStore(ms, note.UID), nil
}

// ListNoteShares lists all share links for a note.
// Only the note's creator may call this.
func (s *APIService) ListNoteShares(ctx context.Context, request *apipb.ListNoteSharesRequest) (*apipb.ListNoteSharesResponse, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	if err := s.requireAssignedNoteWritable(ctx, note, user); err != nil {
		return nil, err
	}

	shares, err := s.Store.ListNoteShares(ctx, &store.FindNoteShare{NoteID: &note.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list note shares")
	}

	response := &apipb.ListNoteSharesResponse{}
	for _, ms := range shares {
		response.NoteShares = append(response.NoteShares, convertNoteShareFromStore(ms, note.UID))
	}
	return response, nil
}

// DeleteNoteShare revokes a share link.
// Only the note's creator may call this.
func (s *APIService) DeleteNoteShare(ctx context.Context, request *apipb.DeleteNoteShareRequest) (*emptypb.Empty, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	// name format: notes/{noteUID}/shares/{shareToken}
	tokens, err := GetNameParentTokens(request.Name, NoteNamePrefix, NoteShareNamePrefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid share name: %v", err)
	}
	noteUID, shareToken := tokens[0], tokens[1]

	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	ms, err := s.Store.GetNoteShare(ctx, &store.FindNoteShare{UID: &shareToken})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note share")
	}
	if ms == nil || ms.NoteID != note.ID {
		return nil, status.Errorf(codes.NotFound, "note share not found")
	}

	if err := s.Store.DeleteNoteShare(ctx, &store.DeleteNoteShare{
		UID:    &shareToken,
		NoteID: &note.ID,
		Policy: noteWritePolicy(user.ID, false),
	}); err != nil {
		return nil, mapNoteWriteError(err, "failed to delete note share")
	}
	return &emptypb.Empty{}, nil
}

// GetSharedNote resolves a share token to its note. No authentication required.
// Returns NOT_FOUND for invalid or expired tokens (no information leakage).
func (s *APIService) GetSharedNote(ctx context.Context, request *apipb.GetSharedNoteRequest) (*apipb.Note, error) {
	ms, err := s.getActiveNoteShare(ctx, request.ShareToken)
	if err != nil {
		return nil, err
	}

	note, err := s.Store.GetNote(ctx, &store.FindNote{ID: &ms.NoteID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	// Treat archived or missing notes the same as an invalid token — no information leakage.
	if note == nil || note.RowStatus != store.Normal || note.Visibility == store.SpaceAudience {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	readContext, err := s.buildNoteReadContext(ctx, note, &ms.NoteID)
	if err != nil || !access.CheckNoteReadContext(readContext).Allowed() {
		return nil, status.Error(codes.NotFound, "not found")
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteID: &note.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}

	noteMessage, err := s.convertNoteFromStore(ctx, note, reactions, attachments, nil)
	if err != nil {
		if stderrors.Is(err, errNoteCreatorNotFound) {
			return nil, status.Errorf(codes.NotFound, "not found")
		}
		return nil, errors.Wrap(err, "failed to convert note")
	}
	// A share token grants access to this note only, not to its surrounding
	// conversation or relation graph.
	noteMessage.Parent = nil
	return noteMessage, nil
}

// isNoteShareExpired returns true if the share has a defined expiry that has already passed.
func isNoteShareExpired(ms *store.NoteShare) bool {
	return ms.ExpiresTs != nil && time.Now().Unix() > *ms.ExpiresTs
}

func (s *APIService) getActiveNoteShare(ctx context.Context, shareID string) (*store.NoteShare, error) {
	ms, err := s.Store.GetNoteShare(ctx, &store.FindNoteShare{UID: &shareID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note share")
	}
	if ms == nil || isNoteShareExpired(ms) {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return ms, nil
}

// convertNoteShareFromStore converts a store NoteShare to the proto NoteShare message.
// name format: notes/{noteUID}/shares/{shareToken}.
func convertNoteShareFromStore(ms *store.NoteShare, noteUID string) *apipb.NoteShare {
	name := fmt.Sprintf("%s%s/%s%s", NoteNamePrefix, noteUID, NoteShareNamePrefix, ms.UID)
	pb := &apipb.NoteShare{
		Name:       name,
		CreateTime: timestamppb.New(time.Unix(ms.CreatedTs, 0)),
	}
	if ms.ExpiresTs != nil {
		pb.ExpireTime = timestamppb.New(time.Unix(*ms.ExpiresTs, 0))
	}
	return pb
}

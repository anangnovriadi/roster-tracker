package api

import (
	"context"
	stderrors "errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) ListNoteReactions(ctx context.Context, request *apipb.ListNoteReactionsRequest) (*apipb.ListNoteReactionsResponse, error) {
	// Extract note UID and check visibility.
	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note: %v", err)
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}

	response := &apipb.ListNoteReactionsResponse{
		Reactions: []*apipb.Reaction{},
	}
	response.Reactions, err = s.convertReactionsFromStoreWithCreators(ctx, reactions, nil, buildNoteName(note.UID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert reactions")
	}
	return response, nil
}

func (s *APIService) UpsertNoteReaction(ctx context.Context, request *apipb.UpsertNoteReactionRequest) (*apipb.Reaction, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}
	if request.GetReaction() == nil {
		return nil, status.Error(codes.InvalidArgument, "reaction is required")
	}

	// Extract note UID and check visibility before allowing reaction.
	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note: %v", err)
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}
	// Each member reacts to a note at most once; removing the reaction frees the slot.
	existing, err := s.Store.ListReactions(ctx, &store.FindReaction{CreatorID: &user.ID, NoteID: &note.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions: %v", err)
	}
	if len(existing) > 0 {
		return nil, status.Errorf(codes.AlreadyExists, "you have already reacted to this note")
	}
	reaction, err := s.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: request.Reaction.ReactionType,
		Policy:       reactionWritePolicy(user.ID),
	})
	if err != nil {
		return nil, mapReactionMutationError(err, "failed to upsert reaction")
	}

	noteName := buildNoteName(note.UID)
	reactionMessage, err := s.convertReactionFromStore(ctx, reaction, noteName)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert reaction")
	}

	s.SSEHub.publishNoteChanged()

	return reactionMessage, nil
}

func (s *APIService) DeleteNoteReaction(ctx context.Context, request *apipb.DeleteNoteReactionRequest) (*emptypb.Empty, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	noteUID, reactionID, err := ExtractNoteReactionIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid reaction name: %v", err)
	}

	// Get reaction and check ownership.
	reaction, err := s.Store.GetReaction(ctx, &store.FindReaction{
		ID: &reactionID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get reaction")
	}
	if reaction == nil {
		// Return permission denied to avoid revealing if reaction exists.
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	if reaction.CreatorID != user.ID && !access.IsInstanceAdmin(user) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{ID: &reaction.NoteID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil || note.UID != noteUID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	if err := s.Store.DeleteReaction(ctx, &store.DeleteReaction{
		ID:          &reactionID,
		NoteID:      &reaction.NoteID,
		ActorUserID: &user.ID,
		Policy:      reactionWritePolicy(user.ID),
	}); err != nil {
		return nil, mapReactionMutationError(err, "failed to delete reaction")
	}

	s.SSEHub.publishNoteChanged()

	return &emptypb.Empty{}, nil
}

func reactionWritePolicy(actorUserID int32) *store.ReactionWritePolicy {
	return &store.ReactionWritePolicy{ActorUserID: actorUserID}
}

func mapReactionMutationError(err error, operation string) error {
	switch {
	case stderrors.Is(err, store.ErrReactionNoteNotFound):
		return status.Error(codes.NotFound, "note not found")
	case stderrors.Is(err, store.ErrReactionPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	default:
		return mapNoteWriteError(err, operation)
	}
}

func (s *APIService) convertReactionFromStore(ctx context.Context, reaction *store.Reaction, noteName string) (*apipb.Reaction, error) {
	creatorsByID, err := s.listUsersByIDWithExisting(ctx, []int32{reaction.CreatorID}, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get reaction creator")
	}
	reactionMessage, err := convertReactionFromStoreWithCreators(reaction, creatorsByID, noteName)
	if err != nil {
		slog.Warn("Failed to convert reaction with missing creator",
			slog.Int64("reaction_id", int64(reaction.ID)),
			slog.Int64("creator_id", int64(reaction.CreatorID)),
			slog.Int64("note_id", int64(reaction.NoteID)),
		)
		return nil, status.Errorf(codes.NotFound, "reaction creator not found")
	}
	return reactionMessage, nil
}

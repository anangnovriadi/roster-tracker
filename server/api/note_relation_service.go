package api

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) SetNoteRelations(ctx context.Context, request *apipb.SetNoteRelationsRequest) (*emptypb.Empty, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := requireNoteWriter(user); err != nil {
		return nil, err
	}
	noteUID, err := ExtractNoteUIDFromName(request.Name)
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
	relations, err := s.prepareNoteRelations(ctx, note, request.Relations)
	if err != nil {
		return nil, err
	}
	updatedTsSec := time.Now().Unix()
	if err := s.applyNoteMutation(ctx, note, nil, &store.UpdateNote{ID: note.ID, UpdatedTs: &updatedTsSec}, nil, &relations); err != nil {
		return nil, err
	}
	_, _, noteMessage, err := s.buildUpdatedNoteState(ctx, note.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to build updated note state")
	}
	s.dispatchNoteUpdatedSideEffects(ctx, noteMessage)

	return &emptypb.Empty{}, nil
}

func (s *APIService) prepareNoteRelations(ctx context.Context, note *store.Note, relations []*apipb.NoteRelation) ([]*store.NoteRelation, error) {
	prepared := make([]*store.NoteRelation, 0, len(relations))
	seenRelatedNoteIDs := make(map[int32]struct{}, len(relations))
	for _, relation := range relations {
		if relation == nil || relation.RelatedNote == nil {
			return nil, status.Errorf(codes.InvalidArgument, "related note is required")
		}
		switch relation.Type {
		case apipb.NoteRelation_REFERENCE:
			// REFERENCE is the only client-mutable relation type.
		case apipb.NoteRelation_COMMENT:
			// COMMENT context is immutable and may only be created atomically with a
			// new note through CreateNoteComment.
			return nil, status.Errorf(codes.InvalidArgument, "COMMENT relations are output only")
		default:
			return nil, status.Errorf(codes.InvalidArgument, "relation type must be REFERENCE")
		}
		// Ignore reflexive relations.
		if buildNoteName(note.UID) == relation.RelatedNote.Name {
			continue
		}
		relatedNoteUID, err := ExtractNoteUIDFromName(relation.RelatedNote.Name)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid related note name: %v", err)
		}
		relatedNote, err := s.Store.GetNote(ctx, &store.FindNote{UID: &relatedNoteUID})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get related note")
		}
		if relatedNote == nil {
			return nil, status.Errorf(codes.NotFound, "related note not found")
		}
		if err := s.checkNoteReadAccess(ctx, relatedNote); err != nil {
			return nil, err
		}
		if _, ok := seenRelatedNoteIDs[relatedNote.ID]; ok {
			continue
		}
		seenRelatedNoteIDs[relatedNote.ID] = struct{}{}
		prepared = append(prepared, &store.NoteRelation{
			NoteID:        note.ID,
			RelatedNoteID: relatedNote.ID,
			Type:          convertNoteRelationTypeToStore(relation.Type),
		})
	}
	return prepared, nil
}

func (s *APIService) ListNoteRelations(ctx context.Context, request *apipb.ListNoteRelationsRequest) (*apipb.ListNoteRelationsResponse, error) {
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
	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}
	relationMap, err := s.batchConvertNoteRelations(ctx, []*store.Note{note}, true)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list note relations: %v", err)
	}
	relationList := relationMap[note.ID]
	if relationList == nil {
		relationList = []*apipb.NoteRelation{}
	}
	limit := normalizePageSize(request.PageSize)
	offset := 0
	if request.PageToken != "" {
		var token apipb.PageToken
		if err := unmarshalPageToken(request.PageToken, &token); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = normalizePageSize(token.Limit)
		offset = max(int(token.Offset), 0)
	}
	end := min(offset+limit, len(relationList))
	if offset > len(relationList) {
		offset = len(relationList)
	}
	nextPageToken := ""
	if end < len(relationList) {
		nextPageToken, err = getPageToken(limit, end)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create page token")
		}
	}

	response := &apipb.ListNoteRelationsResponse{
		Relations:     relationList[offset:end],
		NextPageToken: nextPageToken,
	}
	return response, nil
}

func convertNoteRelationTypeFromStore(relationType store.NoteRelationType) apipb.NoteRelation_Type {
	switch relationType {
	case store.NoteRelationReference:
		return apipb.NoteRelation_REFERENCE
	case store.NoteRelationComment:
		return apipb.NoteRelation_COMMENT
	default:
		return apipb.NoteRelation_TYPE_UNSPECIFIED
	}
}

func convertNoteRelationTypeToStore(relationType apipb.NoteRelation_Type) store.NoteRelationType {
	switch relationType {
	case apipb.NoteRelation_COMMENT:
		return store.NoteRelationComment
	default:
		return store.NoteRelationReference
	}
}

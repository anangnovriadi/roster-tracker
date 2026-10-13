package store

import (
	"context"
	"errors"
)

type NoteRelationType string

const (
	// NoteRelationReference is the type for a reference note relation.
	NoteRelationReference NoteRelationType = "REFERENCE"
	// NoteRelationComment is the type for a comment note relation.
	NoteRelationComment NoteRelationType = "COMMENT"
)

type NoteRelation struct {
	NoteID        int32
	RelatedNoteID int32
	Type          NoteRelationType
}

// NoteRelationEndpointSnapshot is the note-local database state used to
// authorize one endpoint during a reference-relation mutation.
type NoteRelationEndpointSnapshot struct {
	ActorUserID int32
	Actor       NoteActorState

	EndpointID         int32
	EndpointCreatorID  int32
	EndpointRowStatus  RowStatus
	EndpointVisibility Visibility
	EndpointSpaceID    *int32

	EndpointSpaceExists  bool
	EndpointMemberActive bool
}

// ValidateNoteRelationEndpointRead applies authenticated note-local read
// policy to an endpoint. An active instance administrator passes the audience
// checks; structural validity still applies.
func ValidateNoteRelationEndpointRead(snapshot *NoteRelationEndpointSnapshot) error {
	if snapshot == nil || !snapshot.Actor.Active {
		return ErrNotePermissionDenied
	}
	if snapshot.EndpointID <= 0 {
		return ErrNoteMutationConflict
	}
	if snapshot.EndpointRowStatus != Normal && snapshot.EndpointRowStatus != Archived {
		return ErrNoteMutationConflict
	}
	if !isValidVisibility(snapshot.EndpointVisibility) {
		return ErrNoteMutationConflict
	}
	if snapshot.EndpointVisibility == SpaceAudience &&
		(snapshot.EndpointSpaceID == nil || !snapshot.EndpointSpaceExists) {
		return ErrNoteMutationConflict
	}
	if snapshot.Actor.Admin {
		return nil
	}
	if snapshot.EndpointRowStatus == Archived && snapshot.EndpointCreatorID != snapshot.ActorUserID {
		return ErrNotePermissionDenied
	}

	switch snapshot.EndpointVisibility {
	case Public, Protected:
		return nil
	case Private:
		if snapshot.EndpointCreatorID != snapshot.ActorUserID {
			return ErrNotePermissionDenied
		}
		return nil
	case SpaceAudience:
		if !snapshot.EndpointMemberActive {
			return ErrNotePermissionDenied
		}
		return nil
	default:
		return ErrNoteMutationConflict
	}
}

type FindNoteRelation struct {
	NoteID        *int32
	RelatedNoteID *int32
	Type          *NoteRelationType
	NoteFilter    *string
	// SourceNoteRowStatus filters the relation source note before pagination.
	SourceNoteRowStatus *RowStatus
	// NoteIDList matches relations where note_id OR related_note_id is in the list.
	NoteIDList []int32
	// SourceNoteIDList matches relations where note_id is in the list.
	SourceNoteIDList []int32
	// RelatedNoteIDList matches relations where related_note_id is in the list.
	RelatedNoteIDList []int32
	Limit             *int
	Offset            *int
}

type DeleteNoteRelation struct {
	NoteID        *int32
	RelatedNoteID *int32
	Type          *NoteRelationType
}

// ValidateNoteRelationDelete ensures the generic deletion path can only
// mutate reference relations. COMMENT relations are immutable context and are
// removed only as part of the lifecycle of an incident note.
func ValidateNoteRelationDelete(delete *DeleteNoteRelation) error {
	if delete == nil {
		return errors.New("note relation deletion is required")
	}
	if delete.Type == nil || *delete.Type != NoteRelationReference {
		return errors.New("only REFERENCE note relations may be deleted")
	}
	if delete.NoteID == nil && delete.RelatedNoteID == nil {
		return errors.New("note relation deletion requires an endpoint")
	}
	if (delete.NoteID != nil && *delete.NoteID <= 0) || (delete.RelatedNoteID != nil && *delete.RelatedNoteID <= 0) {
		return errors.New("note relation deletion has an invalid endpoint")
	}
	return nil
}

func (s *Store) UpsertNoteRelation(ctx context.Context, create *NoteRelation) (*NoteRelation, error) {
	if create == nil || create.Type != NoteRelationReference {
		return nil, errors.New("only REFERENCE note relations may be persisted")
	}
	return s.driver.UpsertNoteRelation(ctx, create)
}

func (s *Store) ListNoteRelations(ctx context.Context, find *FindNoteRelation) ([]*NoteRelation, error) {
	return s.driver.ListNoteRelations(ctx, find)
}

func (s *Store) DeleteNoteRelation(ctx context.Context, delete *DeleteNoteRelation) error {
	if err := ValidateNoteRelationDelete(delete); err != nil {
		return err
	}
	return s.driver.DeleteNoteRelation(ctx, delete)
}

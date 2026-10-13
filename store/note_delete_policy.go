package store

import (
	"context"

	"github.com/pkg/errors"
)

// DeleteNoteWithPolicy identifies one note to delete atomically on behalf of
// its author or an instance administrator. The note's comment subtree
// (comments and their nested replies) is deleted with it; other relations do
// not confer lifecycle authority.
type DeleteNoteWithPolicy struct {
	NoteID      int32
	ActorUserID int32
}

// DeleteNoteWithPolicyResult contains authorization state captured by the
// deletion transaction.
type DeleteNoteWithPolicyResult struct {
	ActorCanRead bool
	Attachments  []*Attachment
}

// NoteDeleteActorCanRead applies the note-local audience matrix after the
// deletion transaction has established that the actor is the active author or
// an instance administrator, who counts as a member everywhere. Invalid
// lifecycle or audience state fails closed. Placement validity matters only
// for the Space audience; it is not an extra read gate for other values.
func NoteDeleteActorCanRead(rowStatus RowStatus, visibility Visibility, spaceID *int32, spaceExists, actorMember bool) bool {
	if rowStatus != Normal && rowStatus != Archived {
		return false
	}
	switch visibility {
	case Public, Protected, Private:
		return true
	case SpaceAudience:
		return spaceID != nil && spaceExists && actorMember
	default:
		return false
	}
}

// DeleteNoteWithPolicy atomically deletes one note, its whole comment
// subtree, their owned database resources, and relations for which any of
// them is an endpoint.
func (s *Store) DeleteNoteWithPolicy(ctx context.Context, delete *DeleteNoteWithPolicy) (*DeleteNoteWithPolicyResult, error) {
	if delete == nil || delete.NoteID <= 0 || delete.ActorUserID <= 0 {
		return nil, errors.New("note deletion requires note and actor")
	}
	result, err := s.driver.DeleteNoteWithPolicy(ctx, delete)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("unexpected nil note deletion result")
	}
	return result, nil
}

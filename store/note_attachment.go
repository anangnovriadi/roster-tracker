package store

import (
	"context"
	"errors"
	"slices"
)

// ErrNoteMutationConflict indicates that note or attachment state changed
// after an API request prepared its mutation.
var ErrNoteMutationConflict = errors.New("note state changed")

// NoteAttachmentBinding describes one attachment that should be bound to a
// note. WasBoundToNote distinguishes an existing binding from a new one so the
// driver can reject ownership transfers while preserving legacy rows already
// attached to the note. A new binding must be owned by the note author, or by
// the acting instance administrator.
type NoteAttachmentBinding struct {
	ID             int32
	UID            string
	UpdatedTs      int64
	WasBoundToNote bool
}

// NoteMutation atomically updates a note, its attachment bindings, and its
// reference relations. Removed attachment rows are deleted in the same
// transaction; callers may clean up external objects after the commit.
type NoteMutation struct {
	// NoteCreate requests that the note row be created in this same transaction
	// before attachments and relations are applied. CommentContextNoteID creates
	// one immutable COMMENT relation from the new note to the named context note.
	NoteCreate                *Note
	CommentContextNoteID      *int32
	NoteID                    int32
	NoteCreatorID             int32
	ExpectedNoteContent       string
	NoteUpdate                *UpdateNote
	Bindings                  []*NoteAttachmentBinding
	RemovedAttachmentIDs      []int32
	RequiredAttachmentIDs     []int32
	ReplaceReferenceRelations bool
	ReferenceRelations        []*NoteRelation
	Policy                    *NoteWritePolicy
}

// NoteAttachmentBindingOwnerAllowed reports whether an unbound attachment may
// be bound to a note: it must be owned by the note author, or by the acting
// instance administrator. Ownership is never transferred between accounts.
func NoteAttachmentBindingOwnerAllowed(attachmentCreatorID, noteCreatorID, actorUserID int32, actorIsAdmin bool) bool {
	if attachmentCreatorID == noteCreatorID {
		return true
	}
	return actorIsAdmin && attachmentCreatorID == actorUserID
}

// NoteAttachmentRemovalAllowed reports whether an attachment may be removed
// from a note. Removal deletes the attachment, so it must be bound to that
// note and owned by the actor unless the actor is an instance administrator.
func NoteAttachmentRemovalAllowed(attachment *Attachment, noteID, actorUserID int32, actorIsAdmin bool) bool {
	if attachment == nil || attachment.NoteID == nil || *attachment.NoteID != noteID {
		return false
	}
	return actorIsAdmin || attachment.CreatorID == actorUserID
}

// WritePolicy returns the transport-facing policy governing the mutation, if
// any: the mutation's own policy, else the one carried by its note update.
func (m *NoteMutation) WritePolicy() *NoteWritePolicy {
	if m.Policy != nil {
		return m.Policy
	}
	if m.NoteUpdate != nil {
		return m.NoteUpdate.Policy
	}
	return nil
}

// ActorUserID returns the user performing the mutation. Transport-facing
// updates carry the actor in their policy, which may differ from the note
// author when an instance administrator acts; creations act as the author.
func (m *NoteMutation) ActorUserID() int32 {
	if policy := m.WritePolicy(); policy != nil {
		return policy.ActorUserID
	}
	return m.NoteCreatorID
}

// ApplyNoteMutation atomically applies note fields, attachment bindings, and
// reference relations after rechecking the state used by API validation.
func (s *Store) ApplyNoteMutation(ctx context.Context, mutation *NoteMutation) error {
	if mutation == nil {
		return errors.New("note mutation is required")
	}
	if mutation.NoteCreate != nil {
		if mutation.NoteUpdate != nil || mutation.Policy != nil || len(mutation.RemovedAttachmentIDs) != 0 {
			return errors.New("note creation mutation cannot update or remove existing state")
		}
		create := mutation.NoteCreate
		if create.CreatorID <= 0 {
			return errors.New("note creation mutation requires creator")
		}
		if err := validateNoteCreate(create); err != nil {
			return err
		}
		if mutation.CommentContextNoteID != nil && *mutation.CommentContextNoteID <= 0 {
			return errors.New("comment creation mutation requires a valid context note")
		}
		mutation.NoteCreatorID = create.CreatorID
		mutation.ExpectedNoteContent = create.Content
	}
	policy := mutation.WritePolicy()
	if err := validateNoteWritePolicy(policy); err != nil {
		return err
	}
	mutation.RemovedAttachmentIDs = slices.Clone(mutation.RemovedAttachmentIDs)
	slices.Sort(mutation.RemovedAttachmentIDs)
	removedAttachmentIDs := make(map[int32]struct{}, len(mutation.RemovedAttachmentIDs))
	for index, attachmentID := range mutation.RemovedAttachmentIDs {
		if attachmentID <= 0 {
			return errors.New("removed attachment IDs must be positive")
		}
		if index > 0 && mutation.RemovedAttachmentIDs[index-1] == attachmentID {
			return errors.New("duplicate removed attachment ID")
		}
		removedAttachmentIDs[attachmentID] = struct{}{}
	}
	for _, binding := range mutation.Bindings {
		if binding == nil {
			return errors.New("attachment binding is required")
		}
		if _, removed := removedAttachmentIDs[binding.ID]; removed {
			return errors.New("attachment cannot be both bound and removed")
		}
	}
	for _, attachmentID := range mutation.RequiredAttachmentIDs {
		if _, removed := removedAttachmentIDs[attachmentID]; removed {
			return errors.New("attachment cannot be both required and removed")
		}
	}
	if policy != nil && policy.LifecycleOnly &&
		(len(mutation.Bindings) != 0 || len(mutation.RemovedAttachmentIDs) != 0 || len(mutation.RequiredAttachmentIDs) != 0 || mutation.ReplaceReferenceRelations) {
		return ErrNoteSpaceMembershipRequired
	}
	if mutation.ReplaceReferenceRelations {
		if mutation.NoteCreatorID <= 0 {
			return errors.New("reference relation mutation requires an actor")
		}
		seenRelatedNoteIDs := make(map[int32]struct{}, len(mutation.ReferenceRelations))
		for _, relation := range mutation.ReferenceRelations {
			if relation == nil || relation.RelatedNoteID <= 0 || relation.Type != NoteRelationReference {
				return errors.New("only REFERENCE note relations may be mutated")
			}
			if mutation.NoteCreate == nil && relation.NoteID != mutation.NoteID {
				return errors.New("reference relation source does not match note mutation")
			}
			if mutation.NoteID > 0 && relation.RelatedNoteID == mutation.NoteID {
				return errors.New("reflexive note relations are not allowed")
			}
			if _, ok := seenRelatedNoteIDs[relation.RelatedNoteID]; ok {
				return errors.New("duplicate note reference relation")
			}
			seenRelatedNoteIDs[relation.RelatedNoteID] = struct{}{}
		}
	}
	return s.driver.ApplyNoteMutation(ctx, mutation)
}

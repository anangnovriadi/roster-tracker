package store

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
)

// AttachmentDeletionPolicy captures the caller and the note content observed
// while validating a transport-facing attachment deletion. Drivers compare the
// snapshots with current note state in the deletion transaction.
type AttachmentDeletionPolicy struct {
	ActorUserID          int32
	ExpectedNoteContents map[int32]string
}

// DeleteAttachmentsWithPolicy atomically authorizes and removes the selected
// attachment rows. External object cleanup remains the caller's responsibility
// after the transaction commits.
func (s *Store) DeleteAttachmentsWithPolicy(ctx context.Context, policy *AttachmentDeletionPolicy, attachmentIDs []int32) error {
	normalizedPolicy, normalizedIDs, err := normalizeAttachmentDelete(policy, attachmentIDs)
	if err != nil {
		return err
	}
	return s.driver.DeleteAttachmentsWithPolicy(ctx, normalizedPolicy, normalizedIDs)
}

func normalizeAttachmentDelete(policy *AttachmentDeletionPolicy, attachmentIDs []int32) (*AttachmentDeletionPolicy, []int32, error) {
	if policy == nil || policy.ActorUserID <= 0 || len(attachmentIDs) == 0 {
		return nil, nil, errors.New("attachment deletion requires policy and attachment IDs")
	}
	ids := slices.Clone(attachmentIDs)
	slices.Sort(ids)
	for index, id := range ids {
		if id <= 0 {
			return nil, nil, errors.New("attachment deletion IDs must be positive")
		}
		if index > 0 && ids[index-1] == id {
			return nil, nil, errors.New("duplicate attachment deletion ID")
		}
	}
	expectedNoteContents := maps.Clone(policy.ExpectedNoteContents)
	for noteID := range expectedNoteContents {
		if noteID <= 0 {
			return nil, nil, errors.New("attachment deletion note IDs must be positive")
		}
	}
	return &AttachmentDeletionPolicy{
		ActorUserID:          policy.ActorUserID,
		ExpectedNoteContents: expectedNoteContents,
	}, ids, nil
}

// ValidateAttachmentDeletionNoteSnapshots requires exactly one expected
// content snapshot for every note currently bound to the selected attachments.
func ValidateAttachmentDeletionNoteSnapshots(noteIDs []int32, expectedNoteContents map[int32]string) error {
	if len(noteIDs) != len(expectedNoteContents) {
		return ErrNoteMutationConflict
	}
	for _, noteID := range noteIDs {
		if _, ok := expectedNoteContents[noteID]; !ok {
			return ErrNoteMutationConflict
		}
	}
	return nil
}

// ValidateAttachmentMutationTargets verifies attachment rows used by a
// transport-facing mutation and returns their distinct note bindings in
// ascending order. Every attachment must be owned by the actor unless the
// actor is an instance administrator.
func ValidateAttachmentMutationTargets(actorUserID int32, actorIsAdmin bool, attachmentIDs []int32, attachments []*Attachment) ([]int32, error) {
	if actorUserID <= 0 || len(attachmentIDs) == 0 || len(attachments) != len(attachmentIDs) {
		return nil, ErrNoteMutationConflict
	}
	expectedIDs := make(map[int32]struct{}, len(attachmentIDs))
	for _, id := range attachmentIDs {
		if id <= 0 {
			return nil, ErrNoteMutationConflict
		}
		expectedIDs[id] = struct{}{}
	}
	noteIDs := make(map[int32]struct{})
	seen := make(map[int32]struct{}, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil || attachment.ID <= 0 {
			return nil, ErrNoteMutationConflict
		}
		if _, expected := expectedIDs[attachment.ID]; !expected {
			return nil, ErrNoteMutationConflict
		}
		if _, duplicate := seen[attachment.ID]; duplicate {
			return nil, ErrNoteMutationConflict
		}
		seen[attachment.ID] = struct{}{}
		if !actorIsAdmin && attachment.CreatorID != actorUserID {
			return nil, ErrNotePermissionDenied
		}
		if attachment.NoteID != nil {
			if *attachment.NoteID <= 0 {
				return nil, ErrNoteMutationConflict
			}
			noteIDs[*attachment.NoteID] = struct{}{}
		}
	}
	result := make([]int32, 0, len(noteIDs))
	for noteID := range noteIDs {
		result = append(result, noteID)
	}
	slices.Sort(result)
	return result, nil
}

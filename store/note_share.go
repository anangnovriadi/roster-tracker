package store

import (
	"context"
	"errors"
)

// NoteShare is an access grant that permits read-only access to a note via a bearer token.
type NoteShare struct {
	ID        int32
	UID       string
	NoteID    int32
	CreatorID int32
	CreatedTs int64
	ExpiresTs *int64 // nil means the share never expires
	Policy    *NoteWritePolicy
}

// FindNoteShare is used to filter note shares in list/get queries.
type FindNoteShare struct {
	ID        *int32
	UID       *string
	NoteID    *int32
	CreatorID *int32
}

// DeleteNoteShare identifies a share grant to remove.
type DeleteNoteShare struct {
	ID     *int32
	UID    *string
	NoteID *int32
	// Policy is required for transport-facing revocation and is revalidated in
	// the same transaction as the delete.
	Policy *NoteWritePolicy
}

// CreateNoteShare creates a new share grant.
func (s *Store) CreateNoteShare(ctx context.Context, create *NoteShare) (*NoteShare, error) {
	if create == nil {
		return nil, errors.New("note share is required")
	}
	if err := validateNoteWritePolicy(create.Policy); err != nil {
		return nil, err
	}
	if create.Policy == nil {
		return s.driver.CreateNoteShare(ctx, create)
	}
	return s.driver.CreateNoteShare(ctx, create)
}

// ListNoteShares returns all share grants matching the filter.
func (s *Store) ListNoteShares(ctx context.Context, find *FindNoteShare) ([]*NoteShare, error) {
	return s.driver.ListNoteShares(ctx, find)
}

// GetNoteShare returns the first share grant matching the filter, or nil if none found.
func (s *Store) GetNoteShare(ctx context.Context, find *FindNoteShare) (*NoteShare, error) {
	return s.driver.GetNoteShare(ctx, find)
}

// DeleteNoteShare removes a share grant.
func (s *Store) DeleteNoteShare(ctx context.Context, delete *DeleteNoteShare) error {
	if delete == nil {
		return errors.New("note share deletion is required")
	}
	if delete.ID == nil && delete.UID == nil {
		return errors.New("note share deletion requires id or uid")
	}
	if err := validateNoteWritePolicy(delete.Policy); err != nil {
		return err
	}
	if delete.Policy == nil {
		return s.driver.DeleteNoteShare(ctx, delete)
	}
	if delete.NoteID == nil || *delete.NoteID <= 0 {
		return errors.New("authorized note share deletion requires note")
	}
	return s.driver.DeleteNoteShare(ctx, delete)
}

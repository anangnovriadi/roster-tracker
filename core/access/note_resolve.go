package access

import (
	"context"

	"github.com/anangnovriadi/roster-tracker/store"
)

// NoteReadStore is the store subset needed to resolve a note read context.
// *store.Store satisfies it.
type NoteReadStore interface {
	GetUser(ctx context.Context, find *store.FindUser) (*store.User, error)
	GetSpace(ctx context.Context, find *store.FindSpace) (*store.Space, error)
	GetSpaceMember(ctx context.Context, find *store.FindSpaceMember) (*store.SpaceMember, error)
}

// NoteReadFacts holds the viewer-independent authorization inputs for one note.
// They are resolved once and reused across every viewer evaluated against the
// same note.
type NoteReadFacts struct {
	Note         *store.Note
	CreatorValid bool
	SpaceValid   bool
}

// ResolveNoteReadFacts resolves the note-local authorization inputs that do not
// depend on who is reading: whether the note has a valid creator and, when it is
// assigned, whether its placement still exists. SpaceValid is authorization
// input only for SPACE reads and placement-dependent projection and
// writes; other audiences remain readable through their own note-local rules.
func ResolveNoteReadFacts(ctx context.Context, s NoteReadStore, note *store.Note) (NoteReadFacts, error) {
	facts := NoteReadFacts{Note: note}
	if note == nil {
		return facts, nil
	}
	creatorID := note.CreatorID
	creator, err := s.GetUser(ctx, &store.FindUser{ID: &creatorID})
	if err != nil {
		return NoteReadFacts{}, err
	}
	facts.CreatorValid = creator != nil && creator.ID == creatorID &&
		(creator.RowStatus == store.Normal || creator.RowStatus == store.Archived)

	facts.SpaceValid = note.SpaceID == nil
	if note.SpaceID != nil {
		space, err := s.GetSpace(ctx, &store.FindSpace{ID: note.SpaceID})
		if err != nil {
			return NoteReadFacts{}, err
		}
		facts.SpaceValid = space != nil
	}
	return facts, nil
}

// WithViewer completes the read context for one viewer. Only the membership
// lookup is viewer-dependent, so evaluating additional viewers against the same
// note costs at most one query each.
func (f NoteReadFacts) WithViewer(ctx context.Context, s NoteReadStore, viewer *store.User, allowAnonymous bool, sharedNoteID *int32) (NoteReadContext, error) {
	readContext := NoteReadContext{
		Note:           f.Note,
		Viewer:         viewer,
		AllowAnonymous: allowAnonymous,
		SharedNoteID:   sharedNoteID,
		CreatorValid:   f.CreatorValid,
		SpaceValid:     f.SpaceValid,
	}
	if f.Note == nil || f.Note.SpaceID == nil || !f.SpaceValid {
		return readContext, nil
	}
	// Membership never changes the outcome for an inactive viewer or for an
	// instance administrator, so skip the lookup for both.
	if !IsActiveUser(viewer) || IsInstanceAdmin(viewer) {
		return readContext, nil
	}
	membership, err := s.GetSpaceMember(ctx, &store.FindSpaceMember{SpaceID: f.Note.SpaceID, UserID: &viewer.ID})
	if err != nil {
		return NoteReadContext{}, err
	}
	readContext.ViewerSpaceMember = membership != nil && membership.Role.IsActiveMember()
	return readContext, nil
}

// ResolveNoteReadContext resolves a complete read context for one note and one
// viewer.
func ResolveNoteReadContext(ctx context.Context, s NoteReadStore, note *store.Note, viewer *store.User, allowAnonymous bool, sharedNoteID *int32) (NoteReadContext, error) {
	facts, err := ResolveNoteReadFacts(ctx, s, note)
	if err != nil {
		return NoteReadContext{}, err
	}
	return facts.WithViewer(ctx, s, viewer, allowAnonymous, sharedNoteID)
}

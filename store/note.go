package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/anangnovriadi/roster-tracker/internal/identifier"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
)

// ErrNoteSpaceNotWritable indicates that a note targets a missing or otherwise
// invalid Space.
var ErrNoteSpaceNotWritable = errors.New("note space is not writable")

// ErrNoteSpaceMembershipRequired indicates that the note creator is not an
// active member of its assigned space.
var ErrNoteSpaceMembershipRequired = errors.New("active space membership required")

// ErrNotePermissionDenied indicates that the actor no longer owns the note
// being mutated.
var ErrNotePermissionDenied = errors.New("note mutation permission denied")

// ErrNoteShareConflict indicates that an active share conflicts with a
// SPACE audience transition.
var ErrNoteShareConflict = errors.New("active note share conflicts with audience")

// Visibility is the type of a visibility.
type Visibility string

const (
	// Public is the PUBLIC visibility.
	Public Visibility = "PUBLIC"
	// Protected is the PROTECTED visibility.
	Protected Visibility = "PROTECTED"
	// Private is the PRIVATE visibility.
	Private Visibility = "PRIVATE"
	// SpaceAudience is visible only to active members of its space.
	SpaceAudience Visibility = "SPACE"
)

func (v Visibility) String() string {
	return string(v)
}

type Note struct {
	// ID is the system generated unique identifier for the note.
	ID int32
	// UID is the user defined unique identifier for the note.
	UID string

	// Standard fields
	RowStatus RowStatus
	CreatorID int32
	CreatedTs int64
	UpdatedTs int64

	// Domain specific fields
	Content    string
	Visibility Visibility
	Pinned     bool
	Payload    *storepb.NotePayload
	SpaceID    *int32

	// Composed fields
	ParentUID *string
}

type FindNote struct {
	ID  *int32
	UID *string

	IDList  []int32
	UIDList []string

	// Standard fields
	RowStatus *RowStatus
	CreatorID *int32

	// Domain specific fields
	VisibilityList       []Visibility
	CommentContextNoteID *int32
	Access               *NoteAccessScope
	ExcludeContent       bool
	ExcludeComments      bool
	Filters              []string

	// Pagination
	Limit  *int
	Offset *int

	// Ordering
	OrderByPinned    bool
	OrderByUpdatedTs bool
	OrderByTimeAsc   bool
}

type FindNotePayload struct {
	Raw                *string
	TagSearch          []string
	HasLink            bool
	HasTaskList        bool
	HasCode            bool
	HasIncompleteTasks bool
}

type UpdateNote struct {
	ID         int32
	UID        *string
	CreatedTs  *int64
	UpdatedTs  *int64
	RowStatus  *RowStatus
	Content    *string
	Visibility *Visibility
	Pinned     *bool
	Payload    *storepb.NotePayload
	SpaceID    *int32
	ClearSpace bool
	// Policy is set by transport-facing author mutations. Drivers revalidate it
	// in the same transaction as the update; nil preserves trusted internal and
	// migration callers.
	Policy *NoteWritePolicy
}

// NoteWritePolicy identifies the actor and the intended kind of
// transport-facing mutation. Drivers discover and authorize the current note,
// Space, membership, and share state in the write transaction.
type NoteWritePolicy struct {
	ActorUserID int32
	// LifecycleOnly permits an author who is no longer a member of the source
	// Space to move or withdraw the note. It never permits content, metadata,
	// attachment, relation, or share mutations.
	LifecycleOnly bool
	// CreatingShare rejects a share if the note's current audience is
	// SPACE. Share revocation deliberately leaves this false.
	CreatingShare bool
}

// NoteActorState is the actor's user row state as read inside a mutation
// transaction. Admin marks an active instance administrator; see
// access.IsInstanceAdmin for what that grants. A missing user is the zero
// value.
type NoteActorState struct {
	Active bool
	Admin  bool
}

// NewNoteActorState derives the actor state from a user row. An archived
// administrator is neither active nor privileged.
func NewNoteActorState(rowStatus RowStatus, role Role) NoteActorState {
	active := rowStatus == Normal
	return NoteActorState{Active: active, Admin: active && role == RoleAdmin}
}

// NoteWriteSnapshot is the current database state used to validate a
// NoteWritePolicy. Driver packages populate it in the mutation transaction.
type NoteWriteSnapshot struct {
	// ActorIsAdmin is true when the policy actor is an active instance
	// administrator. It bypasses authorship and membership checks only.
	ActorIsAdmin       bool
	CreatorID          int32
	RowStatus          RowStatus
	SpaceID            *int32
	Visibility         Visibility
	SourceSpaceExists  bool
	SourceMemberActive bool
	TargetSpaceExists  bool
	TargetMemberActive bool
	HasActiveShare     bool
}

// NoteAccessScope is a typed, fail-closed read authorization predicate. When
// present on FindNote, drivers apply it in SQL before pagination.
type NoteAccessScope struct {
	UserID         *int32
	AllowPublic    bool
	AllowProtected bool
}

type DeleteNote struct {
	ID int32
}

func (s *Store) CreateNote(ctx context.Context, create *Note) (*Note, error) {
	if err := validateNoteCreate(create); err != nil {
		return nil, err
	}
	return s.driver.CreateNote(ctx, create)
}

// CreateNoteComment atomically creates one independent note and its immutable
// COMMENT relation to a context note.
func (s *Store) CreateNoteComment(ctx context.Context, create *Note, contextNoteID, actorUserID int32) (*Note, error) {
	if create == nil || contextNoteID <= 0 || actorUserID <= 0 || create.CreatorID != actorUserID {
		return nil, errors.New("comment creation requires note, context, and matching actor")
	}
	if create.Visibility == "" {
		create.Visibility = Private
	}
	if err := s.ApplyNoteMutation(ctx, &NoteMutation{
		NoteCreate:           create,
		CommentContextNoteID: &contextNoteID,
		NoteCreatorID:        actorUserID,
		ExpectedNoteContent:  create.Content,
	}); err != nil {
		return nil, err
	}
	return create, nil
}

func validateNoteCreate(create *Note) error {
	if create == nil {
		return errors.New("note is required")
	}
	if !identifier.UIDMatcher.MatchString(create.UID) {
		return errors.New("invalid uid")
	}
	if !isValidVisibility(create.Visibility) {
		return errors.New("invalid visibility")
	}
	if create.Visibility == SpaceAudience && create.SpaceID == nil {
		return errors.New("SPACE visibility requires a space")
	}
	return nil
}

func (s *Store) ListNotes(ctx context.Context, find *FindNote) ([]*Note, error) {
	return s.driver.ListNotes(ctx, find)
}

// CountUserNotes counts the notes a user created, comments included, in every
// row status and Space.
func (s *Store) CountUserNotes(ctx context.Context, creatorID int32) (int64, error) {
	return s.driver.CountUserNotes(ctx, creatorID)
}

func (s *Store) GetNote(ctx context.Context, find *FindNote) (*Note, error) {
	list, err := s.ListNotes(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	note := list[0]
	return note, nil
}

func (s *Store) UpdateNote(ctx context.Context, update *UpdateNote) error {
	if update.UID != nil && !identifier.UIDMatcher.MatchString(*update.UID) {
		return errors.New("invalid uid")
	}
	if update.Visibility != nil && !isValidVisibility(*update.Visibility) {
		return errors.New("invalid visibility")
	}
	if update.ClearSpace && update.Visibility != nil && *update.Visibility == SpaceAudience {
		return errors.New("SPACE visibility requires a space")
	}
	if err := validateNoteWritePolicy(update.Policy); err != nil {
		return err
	}
	if update.Policy == nil {
		return s.driver.UpdateNote(ctx, update)
	}
	return s.driver.UpdateNote(ctx, update)
}

func validateNoteWritePolicy(policy *NoteWritePolicy) error {
	if policy == nil {
		return nil
	}
	if policy.ActorUserID <= 0 {
		return errors.New("note write policy requires actor")
	}
	if policy.LifecycleOnly && policy.CreatingShare {
		return errors.New("note write policy has conflicting actions")
	}
	return nil
}

// ValidateNoteWriteSnapshot applies the transport-independent write
// invariants to current database state.
func ValidateNoteWriteSnapshot(policy *NoteWritePolicy, update *UpdateNote, snapshot *NoteWriteSnapshot) error {
	if err := validateNoteWritePolicy(policy); err != nil {
		return err
	}
	if policy == nil || snapshot == nil {
		return errors.New("note write policy snapshot is required")
	}
	// An instance administrator is the superuser for named note operations and
	// is not held to authorship or Space membership. Structural validity below
	// still applies to every actor.
	if !snapshot.ActorIsAdmin && snapshot.CreatorID != policy.ActorUserID {
		return ErrNotePermissionDenied
	}
	if snapshot.RowStatus != Normal && snapshot.RowStatus != Archived {
		return ErrNoteMutationConflict
	}
	if policy.CreatingShare && snapshot.RowStatus != Normal {
		return ErrNoteMutationConflict
	}
	if !isValidVisibility(snapshot.Visibility) {
		return ErrNoteMutationConflict
	}
	if policy.LifecycleOnly && !isLifecycleOnlyNoteUpdate(update) {
		// The source-writability exception exists only for an author's explicit
		// move or withdrawal after membership removal. It must never authorize a
		// content or metadata mutation in the same transaction.
		return ErrNoteSpaceMembershipRequired
	}
	if snapshot.SpaceID != nil {
		if !snapshot.SourceSpaceExists {
			return ErrNoteSpaceNotWritable
		}
		if !policy.LifecycleOnly && !snapshot.ActorIsAdmin && !snapshot.SourceMemberActive {
			return ErrNoteSpaceMembershipRequired
		}
	}

	resultSpaceID := snapshot.SpaceID
	resultVisibility := snapshot.Visibility
	if update != nil {
		if update.SpaceID != nil {
			if !snapshot.TargetSpaceExists {
				return ErrNoteSpaceNotWritable
			}
			if !snapshot.ActorIsAdmin && !snapshot.TargetMemberActive {
				return ErrNoteSpaceMembershipRequired
			}
			resultSpaceID = update.SpaceID
		} else if update.ClearSpace {
			resultSpaceID = nil
		}
		if update.Visibility != nil {
			resultVisibility = *update.Visibility
		}
	}
	if !isValidVisibility(resultVisibility) {
		return ErrNoteMutationConflict
	}
	placementChanged := !sameNoteSpace(resultSpaceID, snapshot.SpaceID)
	if placementChanged && snapshot.Visibility == SpaceAudience && (update == nil || update.Visibility == nil) {
		// Moving a current SPACE note changes which membership grants
		// access. Require the audience to be explicitly confirmed in the same
		// mutation, even if a caller read an older audience before the transaction.
		return ErrNoteMutationConflict
	}
	if policy.LifecycleOnly && !placementChanged {
		return ErrNoteSpaceMembershipRequired
	}
	if resultVisibility == SpaceAudience && resultSpaceID == nil {
		return ErrNoteSpaceNotWritable
	}
	if policy.CreatingShare && resultVisibility == SpaceAudience {
		return ErrNoteShareConflict
	}
	if update != nil && update.Visibility != nil && *update.Visibility == SpaceAudience && snapshot.HasActiveShare {
		return ErrNoteShareConflict
	}
	return nil
}

func isLifecycleOnlyNoteUpdate(update *UpdateNote) bool {
	if update == nil || (!update.ClearSpace && update.SpaceID == nil) || (update.ClearSpace && update.SpaceID != nil) {
		return false
	}
	return update.UID == nil &&
		update.CreatedTs == nil &&
		update.UpdatedTs == nil &&
		update.RowStatus == nil &&
		update.Content == nil &&
		update.Pinned == nil &&
		update.Payload == nil
}

// NullInt32Pointer converts a nullable SQL integer to an optional int32.
func NullInt32Pointer(value sql.NullInt64) *int32 {
	if !value.Valid {
		return nil
	}
	result := int32(value.Int64)
	return &result
}

func sameNoteSpace(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func isValidVisibility(visibility Visibility) bool {
	return visibility == Public || visibility == Protected || visibility == Private || visibility == SpaceAudience
}

func (s *Store) DeleteNote(ctx context.Context, delete *DeleteNote) error {
	if delete == nil || delete.ID <= 0 {
		return errors.New("note deletion requires a note")
	}
	note, err := s.GetNote(ctx, &FindNote{ID: &delete.ID})
	if err != nil {
		return err
	}
	if note == nil {
		return nil
	}
	result, err := s.DeleteNoteWithPolicy(ctx, &DeleteNoteWithPolicy{NoteID: note.ID, ActorUserID: note.CreatorID})
	if err != nil {
		return err
	}
	return s.deleteAttachmentStorageSnapshots(ctx, result.Attachments)
}

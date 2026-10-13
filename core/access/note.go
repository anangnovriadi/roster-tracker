// Package access defines transport-independent resource authorization policies
// shared by the server's API and HTTP adapters.
package access

import "github.com/anangnovriadi/roster-tracker/store"

// NoteReadDenial describes why a note read was rejected.
type NoteReadDenial int

const (
	// NoteReadDenialNone means the read is allowed.
	NoteReadDenialNone NoteReadDenial = iota
	// NoteReadDenialNotFound hides missing, archived, and invalid note state.
	NoteReadDenialNotFound
	// NoteReadDenialUnauthenticated means the resource requires a signed-in user.
	NoteReadDenialUnauthenticated
	// NoteReadDenialPermission means the signed-in user cannot read the resource.
	NoteReadDenialPermission
)

// NoteReadClass describes whether the resource is anonymously readable.
type NoteReadClass int

const (
	// NoteReadClassPrivate is for author, authenticated, member, or share-token reads.
	NoteReadClassPrivate NoteReadClass = iota
	// NoteReadClassPublic is for resources currently readable without credentials.
	NoteReadClassPublic
)

// NoteReadDecision is the outcome of evaluating note read access.
type NoteReadDecision struct {
	Denial NoteReadDenial
	Class  NoteReadClass
}

// NoteReadContext contains the fully resolved authorization context for one
// note. Relations never contribute authorization; callers evaluate each
// relation endpoint independently.
type NoteReadContext struct {
	Note              *store.Note
	Viewer            *store.User
	AllowAnonymous    bool
	SharedNoteID      *int32
	CreatorValid      bool
	SpaceValid        bool
	ViewerSpaceMember bool
}

// Allowed reports whether the read is permitted.
func (d NoteReadDecision) Allowed() bool {
	return d.Denial == NoteReadDenialNone
}

// IsActiveUser reports whether the user exists and is in the normal lifecycle
// state, which every authenticated authorization decision requires.
func IsActiveUser(user *store.User) bool {
	return user != nil && user.RowStatus == store.Normal
}

// IsInstanceAdmin reports whether the user is an active application ADMIN.
// An instance administrator is the superuser for named note operations: every
// note-local authorization check (authorship, audience, Space membership and
// participation, attachment and reaction ownership) passes. Structural
// validity still applies, and collection listings keep the audience predicate
// so feeds never surface other users' private notes.
func IsInstanceAdmin(user *store.User) bool {
	return IsActiveUser(user) && user.Role == store.RoleAdmin
}

// CanManageNote reports whether the actor may perform author-level operations
// on the note: the active author, or an instance administrator.
func CanManageNote(actor *store.User, note *store.Note) bool {
	return note != nil && ownsOrAdministers(actor, note.CreatorID)
}

// CanManageAttachment reports whether the actor may mutate an attachment row
// directly: the active owner, or an instance administrator.
func CanManageAttachment(actor *store.User, attachment *store.Attachment) bool {
	return attachment != nil && ownsOrAdministers(actor, attachment.CreatorID)
}

func ownsOrAdministers(actor *store.User, creatorID int32) bool {
	return IsActiveUser(actor) && (actor.ID == creatorID || actor.Role == store.RoleAdmin)
}

// CheckNoteReadContext evaluates access to exactly one note. Unknown audience,
// invalid lifecycle state, and a missing or invalid creator fail closed. A
// dangling placement only invalidates SPACE reads; other audiences
// remain note-local. A share applies only to the exact note and never to either
// endpoint of a relation.
func CheckNoteReadContext(ctx NoteReadContext) NoteReadDecision {
	note := ctx.Note
	if note == nil || !ctx.CreatorValid {
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}
	if note.Visibility != store.Public && note.Visibility != store.Protected && note.Visibility != store.Private && note.Visibility != store.SpaceAudience {
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}
	if note.Visibility == store.SpaceAudience && (note.SpaceID == nil || !ctx.SpaceValid) {
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}

	if note.RowStatus != store.Normal && note.RowStatus != store.Archived {
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}
	// A structurally valid note is readable by name to an instance
	// administrator regardless of audience, placement, or lifecycle state.
	if IsInstanceAdmin(ctx.Viewer) {
		return NoteReadDecision{Class: NoteReadClassPrivate}
	}

	viewerActive := IsActiveUser(ctx.Viewer)
	viewerIsAuthor := viewerActive && ctx.Viewer.ID == note.CreatorID
	if note.RowStatus == store.Archived && !viewerIsAuthor {
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}

	shareApplies := ctx.SharedNoteID != nil && note.ID == *ctx.SharedNoteID && note.Visibility != store.SpaceAudience
	if shareApplies {
		return NoteReadDecision{Class: NoteReadClassPrivate}
	}

	switch note.Visibility {
	case store.Public:
		if ctx.AllowAnonymous {
			return NoteReadDecision{Class: NoteReadClassPublic}
		}
		if viewerActive {
			return NoteReadDecision{Class: NoteReadClassPrivate}
		}
		return NoteReadDecision{Denial: NoteReadDenialUnauthenticated}
	case store.Protected:
		if viewerActive {
			return NoteReadDecision{Class: NoteReadClassPrivate}
		}
		return NoteReadDecision{Denial: NoteReadDenialUnauthenticated}
	case store.Private:
		if !viewerActive {
			return NoteReadDecision{Denial: NoteReadDenialUnauthenticated}
		}
		if !viewerIsAuthor {
			return NoteReadDecision{Denial: NoteReadDenialPermission}
		}
		return NoteReadDecision{Class: NoteReadClassPrivate}
	case store.SpaceAudience:
		if !viewerActive {
			return NoteReadDecision{Denial: NoteReadDenialUnauthenticated}
		}
		if ctx.ViewerSpaceMember {
			return NoteReadDecision{Class: NoteReadClassPrivate}
		}
		return NoteReadDecision{Denial: NoteReadDenialPermission}
	default:
		return NoteReadDecision{Denial: NoteReadDenialNotFound}
	}
}

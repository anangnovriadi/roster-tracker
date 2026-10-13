package store

// NoteCommentAuthorizationSnapshot is the note-local database state read by a
// driver before it creates an immutable COMMENT relation.
type NoteCommentAuthorizationSnapshot struct {
	ActorUserID int32
	Actor       NoteActorState

	ContextID         int32
	ContextCreatorID  int32
	ContextRowStatus  RowStatus
	ContextVisibility Visibility
	ContextSpaceID    *int32

	ContextSpaceExists  bool
	ContextMemberActive bool
}

// ValidateNoteCommentAuthorization validates participation in the context
// note. The replying note is authorized independently by the normal note-create
// policy; no placement or audience is inherited from this snapshot.
func ValidateNoteCommentAuthorization(snapshot *NoteCommentAuthorizationSnapshot) error {
	if snapshot == nil || snapshot.ActorUserID <= 0 || !snapshot.Actor.Active {
		return ErrNotePermissionDenied
	}
	if snapshot.ContextID <= 0 || snapshot.ContextRowStatus != Normal || !isValidVisibility(snapshot.ContextVisibility) {
		return ErrNoteSpaceNotWritable
	}
	if snapshot.ContextSpaceID == nil && snapshot.ContextVisibility == SpaceAudience {
		return ErrNoteSpaceNotWritable
	}
	if snapshot.ContextSpaceID != nil && !snapshot.ContextSpaceExists {
		return ErrNoteSpaceNotWritable
	}
	// An instance administrator participates in any structurally valid note.
	if snapshot.Actor.Admin {
		return nil
	}
	// Participation in an assigned note is narrower than reading it: active
	// membership is required even for PUBLIC and PROTECTED audiences.
	if snapshot.ContextSpaceID != nil && !snapshot.ContextMemberActive {
		return ErrNoteSpaceMembershipRequired
	}

	switch snapshot.ContextVisibility {
	case Private:
		if snapshot.ContextCreatorID != snapshot.ActorUserID {
			return ErrNotePermissionDenied
		}
	case Protected, Public, SpaceAudience:
		// Any audience-specific placement and membership requirements were
		// validated above.
	default:
		return ErrNoteSpaceNotWritable
	}
	return nil
}

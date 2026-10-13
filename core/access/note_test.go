package access

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestCheckNoteReadNoteLocalAudiences(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal}
	other := &store.User{ID: 2, RowStatus: store.Normal, Role: store.RoleUser}
	public := &store.Note{ID: 1, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Public}
	protected := &store.Note{ID: 2, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Protected}
	private := &store.Note{ID: 3, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Private}

	require.Equal(t, NoteReadDecision{Class: NoteReadClassPublic}, CheckNoteReadContext(NoteReadContext{Note: public, AllowAnonymous: true, CreatorValid: true, SpaceValid: true}))
	require.Equal(t, NoteReadDenialUnauthenticated, CheckNoteReadContext(NoteReadContext{Note: public, CreatorValid: true, SpaceValid: true}).Denial)
	require.True(t, CheckNoteReadContext(NoteReadContext{Note: protected, Viewer: other, CreatorValid: true, SpaceValid: true}).Allowed())
	require.True(t, CheckNoteReadContext(NoteReadContext{Note: private, Viewer: owner, CreatorValid: true, SpaceValid: true}).Allowed())
	require.Equal(t, NoteReadDenialPermission, CheckNoteReadContext(NoteReadContext{Note: private, Viewer: other, CreatorValid: true, SpaceValid: true}).Denial)

	shareID := private.ID
	require.True(t, CheckNoteReadContext(NoteReadContext{Note: private, SharedNoteID: &shareID, CreatorValid: true, SpaceValid: true}).Allowed())
}

func TestCheckNoteReadCommentDoesNotInheritContext(t *testing.T) {
	const ownerID int32 = 1
	parentUID := "context-note"
	comment := &store.Note{
		ID:         2,
		CreatorID:  ownerID,
		RowStatus:  store.Normal,
		Visibility: store.Public,
		ParentUID:  &parentUID,
	}

	require.Equal(t, NoteReadDecision{Class: NoteReadClassPublic}, CheckNoteReadContext(NoteReadContext{Note: comment, AllowAnonymous: true, CreatorValid: true, SpaceValid: true}))
	comment.Visibility = store.Private
	require.Equal(t, NoteReadDenialUnauthenticated, CheckNoteReadContext(NoteReadContext{Note: comment, AllowAnonymous: true, CreatorValid: true, SpaceValid: true}).Denial)

	shareID := comment.ID
	require.True(t, CheckNoteReadContext(NoteReadContext{Note: comment, SharedNoteID: &shareID, CreatorValid: true, SpaceValid: true}).Allowed())
}

func TestCheckNoteReadSpaceAudience(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal}
	member := &store.User{ID: 2, RowStatus: store.Normal}
	appAdmin := &store.User{ID: 3, RowStatus: store.Normal, Role: store.RoleAdmin}
	spaceID := int32(7)
	note := &store.Note{ID: 10, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.SpaceAudience, SpaceID: &spaceID}

	base := NoteReadContext{Note: note, CreatorValid: true, SpaceValid: true}
	require.Equal(t, NoteReadDenialUnauthenticated, CheckNoteReadContext(base).Denial)

	base.Viewer = owner
	require.Equal(t, NoteReadDenialPermission, CheckNoteReadContext(base).Denial)

	base.ViewerSpaceMember = true
	require.True(t, CheckNoteReadContext(base).Allowed())

	base.Viewer = member
	base.ViewerSpaceMember = true
	require.True(t, CheckNoteReadContext(base).Allowed())

	base.Viewer = appAdmin
	base.ViewerSpaceMember = false
	require.True(t, CheckNoteReadContext(base).Allowed(), "an instance administrator reads any note by name")

	shareID := note.ID
	base.Viewer = nil
	base.SharedNoteID = &shareID
	require.Equal(t, NoteReadDenialUnauthenticated, CheckNoteReadContext(base).Denial)

	note.SpaceID = nil
	base.Viewer = member
	base.ViewerSpaceMember = true
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(base).Denial)
}

func TestInstanceAdminIsNoteSuperuser(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal, Role: store.RoleUser}
	admin := &store.User{ID: 2, RowStatus: store.Normal, Role: store.RoleAdmin}
	archivedAdmin := &store.User{ID: 3, RowStatus: store.Archived, Role: store.RoleAdmin}
	spaceID := int32(7)

	private := &store.Note{ID: 1, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Private}
	archived := &store.Note{ID: 2, CreatorID: owner.ID, RowStatus: store.Archived, Visibility: store.Public}
	spaceNote := &store.Note{ID: 3, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.SpaceAudience, SpaceID: &spaceID}
	for _, note := range []*store.Note{private, archived, spaceNote} {
		require.Equal(t, NoteReadDecision{Class: NoteReadClassPrivate}, CheckNoteReadContext(NoteReadContext{
			Note: note, Viewer: admin, CreatorValid: true, SpaceValid: true,
		}), "admin reads note %d by name", note.ID)
		require.False(t, CheckNoteReadContext(NoteReadContext{
			Note: note, Viewer: archivedAdmin, CreatorValid: true, SpaceValid: true,
		}).Allowed(), "an archived administrator holds no privilege")
		require.True(t, CanManageNote(admin, note))
		require.False(t, CanManageNote(archivedAdmin, note))
	}

	// Structural validity still applies.
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{
		Note: spaceNote, Viewer: admin, CreatorValid: true, SpaceValid: false,
	}).Denial, "a dangling SPACE placement is not readable, even by an administrator")
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{
		Note: private, Viewer: admin, CreatorValid: false, SpaceValid: true,
	}).Denial)

	require.True(t, CanManageNote(owner, private))
	require.False(t, CanManageNote(&store.User{ID: 4, RowStatus: store.Normal, Role: store.RoleUser}, private))
	attachment := &store.Attachment{ID: 1, CreatorID: owner.ID}
	require.True(t, CanManageAttachment(owner, attachment))
	require.True(t, CanManageAttachment(admin, attachment))
	require.False(t, CanManageAttachment(archivedAdmin, attachment))
}

func TestCheckNoteReadInvalidStateFailsClosed(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal}
	spaceID := int32(7)
	note := &store.Note{ID: 10, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Public, SpaceID: &spaceID}

	require.Equal(t, NoteReadDecision{Class: NoteReadClassPublic}, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, AllowAnonymous: true, CreatorValid: true, SpaceValid: false,
	}), "a dangling placement does not override a non-Space audience")
	note.Visibility = store.Protected
	require.True(t, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, CreatorValid: true, SpaceValid: false,
	}).Allowed(), "PROTECTED remains readable through its own audience")
	note.Visibility = store.Private
	require.True(t, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, CreatorValid: true, SpaceValid: false,
	}).Allowed(), "PRIVATE remains readable by its active author")
	note.Visibility = store.SpaceAudience
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, CreatorValid: true, SpaceValid: false, ViewerSpaceMember: true,
	}).Denial, "SPACE depends on a valid assigned Space")

	note.SpaceID = nil
	note.Visibility = store.Visibility("FUTURE_AUDIENCE")
	shareID := note.ID
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, AllowAnonymous: true, SharedNoteID: &shareID, CreatorValid: true, SpaceValid: true,
	}).Denial)

	note.Visibility = store.Public
	note.RowStatus = store.Archived
	require.True(t, CheckNoteReadContext(NoteReadContext{Note: note, Viewer: owner, CreatorValid: true, SpaceValid: true}).Allowed())
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{Note: note, AllowAnonymous: true, CreatorValid: true, SpaceValid: true}).Denial)
	note.Visibility = store.SpaceAudience
	note.SpaceID = &spaceID
	require.Equal(t, NoteReadDenialPermission, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, CreatorValid: true, SpaceValid: true,
	}).Denial, "an archived SPACE note still requires active membership")
	require.True(t, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, CreatorValid: true, SpaceValid: true, ViewerSpaceMember: true,
	}).Allowed())

	note.RowStatus = store.RowStatus("UNKNOWN")
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{Note: note, Viewer: owner, CreatorValid: true, SpaceValid: true}).Denial)

	note.RowStatus = store.Normal
	note.Visibility = store.Public
	note.SpaceID = nil
	require.Equal(t, NoteReadDenialNotFound, CheckNoteReadContext(NoteReadContext{
		Note: note, Viewer: owner, AllowAnonymous: true, SpaceValid: true,
	}).Denial, "a missing or invalid creator fails closed before audience evaluation")
}

func TestCheckNoteReadAssignedPublicHasNoMembershipGate(t *testing.T) {
	owner := &store.User{ID: 1}
	other := &store.User{ID: 2, RowStatus: store.Normal}
	spaceID := int32(7)
	assigned := &store.Note{ID: 10, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Public, SpaceID: &spaceID}

	require.Equal(t, NoteReadDecision{Class: NoteReadClassPublic}, CheckNoteReadContext(NoteReadContext{
		Note: assigned, AllowAnonymous: true, CreatorValid: true, SpaceValid: true,
	}))
	require.True(t, CheckNoteReadContext(NoteReadContext{
		Note: assigned, Viewer: other, CreatorValid: true, SpaceValid: true,
	}).Allowed(), "placement does not add a read gate to PUBLIC notes")
}

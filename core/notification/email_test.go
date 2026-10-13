package notification

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/internal/profile"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
	teststore "github.com/anangnovriadi/roster-tracker/store/test"
)

func TestEmailNoteReadCreatorLifecycle(t *testing.T) {
	ctx := context.Background()
	st := teststore.NewTestingStore(ctx, t)
	defer st.Close()
	dispatcher := NewEmailDispatcher(nil, st, nil)

	creator, err := st.CreateUser(ctx, &store.User{Username: "email-note-creator", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := st.CreateUser(ctx, &store.User{Username: "email-note-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	note, err := st.CreateNote(ctx, &store.Note{
		UID: "email-note-creator-lifecycle", CreatorID: creator.ID, Content: "note", Visibility: store.Protected,
	})
	require.NoError(t, err)
	archived := store.Archived
	_, err = st.UpdateUser(ctx, &store.UpdateUser{ID: creator.ID, RowStatus: &archived})
	require.NoError(t, err)

	readable, err := dispatcher.listNotesByID(ctx, []int32{note.ID}, viewer.ID)
	require.NoError(t, err)
	require.Contains(t, readable, note.ID, "archiving the creator must not narrow an active PROTECTED note")

	_, err = st.GetDriver().GetDB().ExecContext(ctx,
		fmt.Sprintf("UPDATE note SET creator_id = 2147483000 WHERE id = %d", note.ID))
	require.NoError(t, err)
	danglingNote, err := st.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.NotNil(t, danglingNote)
	readable, err = dispatcher.listNotesByID(ctx, []int32{danglingNote.ID}, viewer.ID)
	require.NoError(t, err)
	require.NotContains(t, readable, danglingNote.ID, "email rendering must fail closed for a missing note creator")
}

func TestEmailSpaceInvitationFailsClosedWithoutPendingOffer(t *testing.T) {
	ctx := context.Background()
	st := teststore.NewTestingStore(ctx, t)
	defer st.Close()
	dispatcher := NewEmailDispatcher(&profile.Profile{InstanceURL: "https://notes.example.com/"}, st, nil)

	inviter, err := st.CreateUser(ctx, &store.User{Username: "email-space-inviter", Nickname: "Ada", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	invitee, err := st.CreateUser(ctx, &store.User{Username: "email-space-invitee", Email: "invitee@example.com", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := st.CreateSpace(ctx, &store.Space{UID: "email-space", Title: "Design team"}, inviter.ID)
	require.NoError(t, err)
	_, err = st.CreateSpaceInvitation(ctx, &store.SpaceInvitation{SpaceID: space.ID, UserID: invitee.ID, Role: store.SpaceMemberRoleAdmin}, inviter.ID)
	require.NoError(t, err)

	inbox := &store.Inbox{
		SenderID:   inviter.ID,
		ReceiverID: invitee.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type:    storepb.InboxMessage_SPACE_INVITATION,
			Payload: &storepb.InboxMessage_SpaceInvitation{SpaceInvitation: &storepb.InboxMessage_SpaceInvitationPayload{SpaceId: space.ID}},
		},
	}
	message, err := dispatcher.buildInboxEmailMessage(ctx, inbox, invitee, inviter, nil)
	require.NoError(t, err)
	require.NotNil(t, message)
	require.Equal(t, []string{"invitee@example.com"}, message.To)
	require.Equal(t, `[Roster Tracker] Ada invited you to join "Design team"`, message.Subject)
	require.Contains(t, message.Body, `Ada invited you to join the Space "Design team" as an administrator.`)
	require.Contains(t, message.Body, "https://notes.example.com/inbox")

	require.NoError(t, st.DeclineSpaceInvitation(ctx, &store.DeclineSpaceInvitation{SpaceID: space.ID, UserID: invitee.ID}, invitee.ID))
	message, err = dispatcher.buildInboxEmailMessage(ctx, inbox, invitee, inviter, nil)
	require.NoError(t, err)
	require.Nil(t, message, "a declined invitation must not be emailed")
}

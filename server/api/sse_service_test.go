package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/anangnovriadi/roster-tracker/internal/profile"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/server/auth"
	"github.com/anangnovriadi/roster-tracker/store"
	teststore "github.com/anangnovriadi/roster-tracker/store/test"
)

// newIntegrationService builds a minimal APIService backed by an in-memory
// SQLite database.  The store is closed automatically via t.Cleanup.
func newIntegrationService(t *testing.T) *APIService {
	t.Helper()
	ctx := context.Background()
	st := teststore.NewTestingStore(ctx, t)
	t.Cleanup(func() { st.Close() })
	p := &profile.Profile{Demo: true, Data: t.TempDir(), Driver: "sqlite", DSN: ":memory:"}
	return NewAPIService("test-secret", p, st)
}

// userCtx returns a context that authenticates as the given user.
func userCtx(ctx context.Context, userID int32) context.Context {
	return context.WithValue(ctx, auth.UserIDContextKey, userID)
}

// collectEventsFor reads events from ch for the given duration and returns them.
func collectEventsFor(ch <-chan []byte, d time.Duration) []string {
	var out []string
	deadline := time.After(d)
	for {
		select {
		case data := <-ch:
			out = append(out, string(data))
		case <-deadline:
			return out
		}
	}
}

func requireNoteChanged(t *testing.T, ch <-chan []byte) {
	t.Helper()
	require.Equal(t, noteChangedSSEFrame, string(mustReceive(t, ch, time.Second)))
}

func requireSpaceChanged(t *testing.T, ch <-chan []byte) {
	t.Helper()
	require.Equal(t, spaceChangedSSEFrame, string(mustReceive(t, ch, time.Second)))
}

func requireNoNoteChanged(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case event := <-ch:
		require.Failf(t, "unexpected SSE event", "received %q", event)
	default:
	}
}

func TestAttachmentMutationsPublishNoteChangedOnlyWhenBound(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "sse-attachment-owner", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "attachment event note", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)
	createAttachment := func(filename string, noteName *string) *apipb.Attachment {
		t.Helper()
		attachment, err := svc.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
			Filename: filename,
			Type:     "text/plain",
			Content:  []byte(filename),
			Note:     noteName,
		}})
		require.NoError(t, err)
		return attachment
	}

	unbound := createAttachment("sse-unbound.txt", nil)
	requireNoNoteChanged(t, client.events)
	_, err = svc.UpdateAttachment(ownerCtx, &apipb.UpdateAttachmentRequest{
		Attachment: &apipb.Attachment{Name: unbound.Name, Filename: "sse-unbound-renamed.txt"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"filename"}},
	})
	require.NoError(t, err)
	requireNoNoteChanged(t, client.events)
	_, err = svc.DeleteAttachment(ownerCtx, &apipb.DeleteAttachmentRequest{Name: unbound.Name})
	require.NoError(t, err)
	requireNoNoteChanged(t, client.events)

	bound := createAttachment("sse-bound.txt", &note.Name)
	requireNoteChanged(t, client.events)
	_, err = svc.UpdateAttachment(ownerCtx, &apipb.UpdateAttachmentRequest{
		Attachment: &apipb.Attachment{Name: bound.Name, Filename: "sse-bound-renamed.txt"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"filename"}},
	})
	require.NoError(t, err)
	requireNoteChanged(t, client.events)
	_, err = svc.DeleteAttachment(ownerCtx, &apipb.DeleteAttachmentRequest{Name: bound.Name})
	require.NoError(t, err)
	requireNoteChanged(t, client.events)

	firstUnbound := createAttachment("sse-batch-unbound-first.txt", nil)
	secondUnbound := createAttachment("sse-batch-unbound-second.txt", nil)
	requireNoNoteChanged(t, client.events)
	_, err = svc.BatchDeleteAttachments(ownerCtx, &apipb.BatchDeleteAttachmentsRequest{
		Names: []string{firstUnbound.Name, secondUnbound.Name},
	})
	require.NoError(t, err)
	requireNoNoteChanged(t, client.events)

	mixedUnbound := createAttachment("sse-batch-mixed-unbound.txt", nil)
	mixedBound := createAttachment("sse-batch-mixed-bound.txt", &note.Name)
	requireNoteChanged(t, client.events)
	_, err = svc.BatchDeleteAttachments(ownerCtx, &apipb.BatchDeleteAttachmentsRequest{
		Names: []string{mixedUnbound.Name, mixedBound.Name},
	})
	require.NoError(t, err)
	requireNoteChanged(t, client.events)
}

func TestUpdateAttachmentPublishesAfterCommittedAccessChange(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "sse-attachment-access-owner", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	space, err := svc.CreateSpace(ownerCtx, &apipb.CreateSpaceRequest{
		SpaceId: "sse-attachment-access-space",
		Space:   &apipb.Space{Title: "Attachment access space"},
	})
	require.NoError(t, err)
	spaceName := space.Name
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "attachment access changed during update",
		Visibility: apipb.Visibility_SPACE,
		Space:      &spaceName,
	}})
	require.NoError(t, err)
	attachment, err := svc.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: "before.txt",
		Type:     "text/plain",
		Content:  []byte("attachment"),
		Note:     &note.Name,
	}})
	require.NoError(t, err)
	attachmentUID, err := ExtractAttachmentUIDFromName(attachment.Name)
	require.NoError(t, err)
	storedAttachment, err := svc.Store.GetAttachment(ctx, &store.FindAttachment{UID: &attachmentUID})
	require.NoError(t, err)
	require.NotNil(t, storedAttachment)

	// Authorization is checked before the attachment update. Simulate membership
	// changing in the same transaction so the committed response can no longer be
	// built through the read-authorized GetAttachment service method.
	trigger := fmt.Sprintf(`
		CREATE TRIGGER revoke_attachment_owner_membership
		AFTER UPDATE OF filename ON attachment
		WHEN NEW.id = %d
		BEGIN
			DELETE FROM space_member WHERE user_id = %d;
		END`, storedAttachment.ID, owner.ID)
	_, err = svc.Store.GetDriver().GetDB().ExecContext(ctx, trigger)
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)
	updated, err := svc.UpdateAttachment(ownerCtx, &apipb.UpdateAttachmentRequest{
		Attachment: &apipb.Attachment{Name: attachment.Name, Filename: "after.txt"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"filename"}},
	})
	require.NoError(t, err)
	require.Equal(t, "after.txt", updated.Filename)
	requireNoteChanged(t, client.events)
	requireNoNoteChanged(t, client.events)

	_, err = svc.GetAttachment(ownerCtx, &apipb.GetAttachmentRequest{Name: attachment.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "the authorized read would fail after the committed membership change")
}

func TestDeleteAttachmentPublishesBeforeStorageCleanupFailure(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "sse-attachment-cleanup-owner", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "attachment cleanup failure", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)
	attachment, err := svc.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: "cleanup.txt",
		Type:     "text/plain",
		Content:  []byte("attachment"),
		Note:     &note.Name,
	}})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)
	_, err = svc.DeleteAttachment(store.WithDeleteAttachmentStorageFailpoint(ownerCtx), &apipb.DeleteAttachmentRequest{Name: attachment.Name})
	require.Equal(t, codes.Internal, status.Code(err))
	require.ErrorContains(t, err, "attachments were deleted but storage cleanup failed")
	requireNoteChanged(t, client.events)
	requireNoNoteChanged(t, client.events)

	attachmentUID, err := ExtractAttachmentUIDFromName(attachment.Name)
	require.NoError(t, err)
	storedAttachment, err := svc.Store.GetAttachment(ctx, &store.FindAttachment{UID: &attachmentUID})
	require.NoError(t, err)
	require.Nil(t, storedAttachment, "the database deletion must remain committed")
}

func TestSpaceMutationsPublishSpaceChanged(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "sse-space-owner", store.RoleAdmin)
	member := createSpaceTestUser(ctx, t, svc, "sse-space-member", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	space, err := svc.CreateSpace(ownerCtx, &apipb.CreateSpaceRequest{
		SpaceId: "sse-space",
		Space:   &apipb.Space{Title: "SSE Space"},
	})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)

	space.Title = "Renamed SSE Space"
	_, err = svc.UpdateSpace(ownerCtx, &apipb.UpdateSpaceRequest{
		Space:      space,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)

	invitation, err := svc.CreateSpaceInvitation(ownerCtx, &apipb.CreateSpaceInvitationRequest{
		Parent:          space.Name,
		SpaceInvitation: &apipb.SpaceInvitation{Invitee: BuildUserName(member.Username), Role: apipb.SpaceMember_USER},
	})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)
	membership, err := svc.AcceptSpaceInvitation(userCtx(ctx, member.ID), &apipb.AcceptSpaceInvitationRequest{Name: invitation.Name})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)

	membership.Role = apipb.SpaceMember_ADMIN
	_, err = svc.UpdateSpaceMember(ownerCtx, &apipb.UpdateSpaceMemberRequest{
		SpaceMember: membership,
		UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"role"}},
	})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)

	_, err = svc.DeleteSpaceMember(ownerCtx, &apipb.DeleteSpaceMemberRequest{Name: membership.Name})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)

	_, err = svc.DeleteSpace(ownerCtx, &apipb.DeleteSpaceRequest{Name: space.Name})
	require.NoError(t, err)
	requireSpaceChanged(t, client.events)
}

func TestUpdateNoteSSEBroadcastsToAllSubscribers(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "audience-owner", Role: store.RoleUser, Email: "audience-owner@example.com",
	})
	require.NoError(t, err)
	ownerCtx := userCtx(ctx, owner.ID)
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "public before update", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	ownerClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(ownerClient)
	otherClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(otherClient)

	_, err = svc.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: note.Name, Visibility: apipb.Visibility_PRIVATE},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"visibility"}},
	})
	require.NoError(t, err)
	requireNoteChanged(t, ownerClient.events)
	requireNoteChanged(t, otherClient.events)

	_, err = svc.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: note.Name, Content: "private after update"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	requireNoteChanged(t, ownerClient.events)
	requireNoteChanged(t, otherClient.events)
}

func TestMoveSpaceAudienceNoteSSEBroadcastsWithoutAudienceCalculation(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "move-audience-owner", store.RoleUser)
	sourceMember := createSpaceTestUser(ctx, t, svc, "move-source-member", store.RoleUser)
	targetMember := createSpaceTestUser(ctx, t, svc, "move-target-member", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)

	source, err := svc.CreateSpace(ownerCtx, &apipb.CreateSpaceRequest{
		SpaceId: "move-source-space",
		Space:   &apipb.Space{Title: "Source"},
	})
	require.NoError(t, err)
	target, err := svc.CreateSpace(ownerCtx, &apipb.CreateSpaceRequest{
		SpaceId: "move-target-space",
		Space:   &apipb.Space{Title: "Target"},
	})
	require.NoError(t, err)
	inviteAndAcceptSpaceTestUser(ctx, t, svc, owner, sourceMember, source, apipb.SpaceMember_USER)
	inviteAndAcceptSpaceTestUser(ctx, t, svc, owner, targetMember, target, apipb.SpaceMember_USER)

	sourceName := source.Name
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "move between spaces",
		Visibility: apipb.Visibility_SPACE,
		Space:      &sourceName,
	}})
	require.NoError(t, err)
	sourceClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(sourceClient)
	targetClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(targetClient)
	outsiderClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(outsiderClient)

	targetName := target.Name
	_, err = svc.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:       note.Name,
			Visibility: apipb.Visibility_SPACE,
			Space:      &targetName,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space", "visibility"}},
	})
	require.NoError(t, err)
	requireNoteChanged(t, sourceClient.events)
	requireNoteChanged(t, targetClient.events)
	requireNoteChanged(t, outsiderClient.events)

	_, err = svc.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: note.Name, Content: "target only now"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	requireNoteChanged(t, sourceClient.events)
	requireNoteChanged(t, targetClient.events)
	requireNoteChanged(t, outsiderClient.events)
}

// ---- CreateNoteComment double-broadcast fix ----

func TestCreateNoteComment_NoDuplicateSSEBroadcast(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	// Create an admin so the store is initialised, then a regular commenter.
	author, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "author", Role: store.RoleUser, Email: "author@example.com",
	})
	require.NoError(t, err)
	commenter, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "commenter", Role: store.RoleUser, Email: "commenter@example.com",
	})
	require.NoError(t, err)

	authorCtx := userCtx(ctx, author.ID)
	commenterCtx := userCtx(ctx, commenter.ID)

	// Create a public note so the commenter can react.
	parent, err := svc.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "parent note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	// Subscribe after the parent note is created so its change event does not
	// pollute the assertion window.
	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	// Comment creation is one mutation and emits one invalidation event.
	_, err = svc.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "a comment", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	// Give the synchronous broadcast a moment to land in the buffer, then
	// collect everything that arrived.
	events := collectEventsFor(client.events, 150*time.Millisecond)

	require.Len(t, events, 1, "expected exactly one SSE event for a comment creation, got: %v", events)
	assert.Equal(t, noteChangedSSEFrame, events[0])
}

func TestCreateNoteComment_SSEBroadcastContainsNoSubject(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	author, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "comment-context-author", Role: store.RoleUser, Email: "comment-context-author@example.com",
	})
	require.NoError(t, err)
	commenter, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "private-comment-author", Role: store.RoleUser, Email: "private-comment-author@example.com",
	})
	require.NoError(t, err)

	parent, err := svc.CreateNote(userCtx(ctx, author.ID), &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "public context", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	authorClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(authorClient)
	commenterClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(commenterClient)

	_, err = svc.CreateNoteComment(userCtx(ctx, commenter.ID), &apipb.CreateNoteCommentRequest{
		Parent: parent.Name,
		Comment: &apipb.Note{
			Content:    "private reply",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)
	requireNoteChanged(t, commenterClient.events)
	requireNoteChanged(t, authorClient.events)
}

func TestCreateNoteWithAttachment_NoDuplicateUpdatedSSEBroadcast(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	user, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "user", Role: store.RoleUser, Email: "user@example.com",
	})
	require.NoError(t, err)
	uctx := userCtx(ctx, user.ID)

	attachment, err := svc.CreateAttachment(uctx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{
			Filename: "test.txt",
			Size:     5,
			Type:     "text/plain",
			Content:  []byte("hello"),
		},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	_, err = svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with initial attachment",
			Visibility: apipb.Visibility_PUBLIC,
			Attachments: []*apipb.Attachment{
				{Name: attachment.Name},
			},
		},
	})
	require.NoError(t, err)

	events := collectEventsFor(client.events, 150*time.Millisecond)

	require.Len(t, events, 1, "expected exactly one SSE event for note creation with attachment, got: %v", events)
	assert.Equal(t, noteChangedSSEFrame, events[0])
}

func TestUpsertNoteReactionPublishesNoteChanged(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	user, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "user", Role: store.RoleUser, Email: "user@example.com",
	})
	require.NoError(t, err)
	uctx := userCtx(ctx, user.ID)

	note, err := svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "reacted note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	_, err = svc.UpsertNoteReaction(uctx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})
	require.NoError(t, err)

	requireNoteChanged(t, client.events)
	mustNotReceive(t, client.events, 100*time.Millisecond)
}

func TestDeleteNoteReactionPublishesNoteChanged(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	user, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "user", Role: store.RoleUser, Email: "user@example.com",
	})
	require.NoError(t, err)
	uctx := userCtx(ctx, user.ID)

	note, err := svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "reacted note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	reaction, err := svc.UpsertNoteReaction(uctx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "❤️",
		},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	_, err = svc.DeleteNoteReaction(uctx, &apipb.DeleteNoteReactionRequest{
		Name: reaction.Name,
	})
	require.NoError(t, err)

	requireNoteChanged(t, client.events)
	mustNotReceive(t, client.events, 100*time.Millisecond)
}

func TestDeleteNote_DeletesCommentSubtreeOnly(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	rootAuthor, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "delete-root-author", Role: store.RoleUser, Email: "delete-root-author@example.com",
	})
	require.NoError(t, err)
	commentAuthor, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "delete-comment-author", Role: store.RoleUser, Email: "delete-comment-author@example.com",
	})
	require.NoError(t, err)
	replyAuthor, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "delete-reply-author", Role: store.RoleUser, Email: "delete-reply-author@example.com",
	})
	require.NoError(t, err)

	root, err := svc.CreateNote(userCtx(ctx, rootAuthor.ID), &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "context note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	comment, err := svc.CreateNoteComment(userCtx(ctx, commentAuthor.ID), &apipb.CreateNoteCommentRequest{
		Parent:  root.Name,
		Comment: &apipb.Note{Content: "branch", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	reply, err := svc.CreateNoteComment(userCtx(ctx, replyAuthor.ID), &apipb.CreateNoteCommentRequest{
		Parent:  comment.Name,
		Comment: &apipb.Note{Content: "nested reply"},
	})
	require.NoError(t, err)

	replyClient := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(replyClient)
	_, err = svc.DeleteNote(userCtx(ctx, commentAuthor.ID), &apipb.DeleteNoteRequest{Name: comment.Name})
	require.NoError(t, err)
	requireNoteChanged(t, replyClient.events)

	commentUID, err := ExtractNoteUIDFromName(comment.Name)
	require.NoError(t, err)
	storedComment, err := svc.Store.GetNote(ctx, &store.FindNote{UID: &commentUID})
	require.NoError(t, err)
	require.Nil(t, storedComment)
	replyUID, err := ExtractNoteUIDFromName(reply.Name)
	require.NoError(t, err)
	storedReply, err := svc.Store.GetNote(ctx, &store.FindNote{UID: &replyUID})
	require.NoError(t, err)
	require.Nil(t, storedReply, "deleting a comment must also delete its replies")
	rootUID, err := ExtractNoteUIDFromName(root.Name)
	require.NoError(t, err)
	storedRoot, err := svc.Store.GetNote(ctx, &store.FindNote{UID: &rootUID})
	require.NoError(t, err)
	require.NotNil(t, storedRoot)
}

func TestDeleteNoteCleansAttachmentStorage(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)
	owner, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "delete-cleanup-owner", Role: store.RoleUser, Email: "delete-cleanup-owner@example.com",
	})
	require.NoError(t, err)
	ownerCtx := userCtx(ctx, owner.ID)
	note, err := svc.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "attachment cleanup", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)
	noteUID, err := ExtractNoteUIDFromName(note.Name)
	require.NoError(t, err)
	storedNote, err := svc.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	require.NoError(t, err)
	require.NotNil(t, storedNote)
	attachmentPath := filepath.Join(t.TempDir(), "sse-delete-cleanup.txt")
	require.NoError(t, os.WriteFile(attachmentPath, []byte("cleanup"), 0o600))
	attachment, err := svc.Store.CreateAttachment(ctx, &store.Attachment{
		UID:         "sse-delete-cleanup",
		CreatorID:   owner.ID,
		Filename:    "cleanup.txt",
		Type:        "text/plain",
		Size:        7,
		Blob:        []byte("cleanup"),
		StorageType: storepb.AttachmentStorageType_LOCAL,
		Reference:   attachmentPath,
		NoteID:      &storedNote.ID,
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)
	_, err = svc.DeleteNote(ownerCtx, &apipb.DeleteNoteRequest{Name: note.Name})
	require.NoError(t, err, "committed deletion must not be reported as a failure")
	requireNoteChanged(t, client.events)

	deletedNote, err := svc.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	require.NoError(t, err)
	require.Nil(t, deletedNote)
	deletedAttachment, err := svc.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Nil(t, deletedAttachment)
	_, err = os.Stat(attachmentPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSetNoteAttachmentsPublishesNoteChanged(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	user, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "user", Role: store.RoleUser, Email: "user@example.com",
	})
	require.NoError(t, err)
	uctx := userCtx(ctx, user.ID)

	note, err := svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "note with attachments", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	attachment, err := svc.CreateAttachment(uctx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{
			Filename: "test.txt",
			Size:     5,
			Type:     "text/plain",
			Content:  []byte("hello"),
		},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	_, err = svc.SetNoteAttachments(uctx, &apipb.SetNoteAttachmentsRequest{
		Name: note.Name,
		Attachments: []*apipb.Attachment{
			{Name: attachment.Name},
		},
	})
	require.NoError(t, err)

	requireNoteChanged(t, client.events)
	mustNotReceive(t, client.events, 100*time.Millisecond)
}

func TestSetNoteRelationsPublishesNoteChanged(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationService(t)

	user, err := svc.Store.CreateUser(ctx, &store.User{
		Username: "user", Role: store.RoleUser, Email: "user@example.com",
	})
	require.NoError(t, err)
	uctx := userCtx(ctx, user.ID)

	note1, err := svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "note one", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	note2, err := svc.CreateNote(uctx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "note two", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	client := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(client)

	_, err = svc.SetNoteRelations(uctx, &apipb.SetNoteRelationsRequest{
		Name: note1.Name,
		Relations: []*apipb.NoteRelation{
			{
				RelatedNote: &apipb.NoteRelation_NoteRef{Name: note2.Name},
				Type:        apipb.NoteRelation_REFERENCE,
			},
		},
	})
	require.NoError(t, err)

	requireNoteChanged(t, client.events)
	mustNotReceive(t, client.events, 100*time.Millisecond)
}

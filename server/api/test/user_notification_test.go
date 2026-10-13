package test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/anangnovriadi/roster-tracker/internal/email"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestListUserNotificationsIncludesNoteCommentPayload(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "notification-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	comment, err := ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Comment content",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", owner.Username),
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)

	notification := resp.Notifications[0]
	require.Contains(t, notification.Name, fmt.Sprintf("users/%s/notifications/", owner.Username))
	require.Equal(t, fmt.Sprintf("users/%s", commenter.Username), notification.Sender)
	require.NotNil(t, notification.SenderUser)
	require.Equal(t, commenter.Username, notification.SenderUser.Username)
	require.Equal(t, apipb.UserNotification_NOTE_COMMENT, notification.Type)
	require.NotNil(t, notification.GetNoteComment())
	require.Equal(t, comment.Name, notification.GetNoteComment().Note)
	require.Equal(t, note.Name, notification.GetNoteComment().RelatedNote)
	require.Equal(t, "Comment content", notification.GetNoteComment().NoteSnippet)
	require.Equal(t, "Base note", notification.GetNoteComment().RelatedNoteSnippet)
}

func TestListUserNotificationsStoresNoteCommentPayloadInInbox(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "notification-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Comment content",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	messageType := storepb.InboxMessage_NOTE_COMMENT
	inboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{
		ReceiverID:  &owner.ID,
		MessageType: &messageType,
	})
	require.NoError(t, err)
	require.Len(t, inboxes, 1)
	require.NotNil(t, inboxes[0].Message)
	require.NotNil(t, inboxes[0].Message.GetNoteComment())
	require.NotZero(t, inboxes[0].Message.GetNoteComment().NoteId)
	require.NotZero(t, inboxes[0].Message.GetNoteComment().RelatedNoteId)
}

func TestAssignedPublicNoteCommentNotifiesReadableRemovedAuthor(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	var sentMessage *email.Message
	ts.Service.NotificationEmailSender = func(_ *email.Config, message *email.Message) {
		sentMessage = message
	}
	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_NOTIFICATION,
		Value: &storepb.InstanceSetting_NotificationSetting{
			NotificationSetting: &storepb.InstanceNotificationSetting{
				Email: &storepb.InstanceNotificationSetting_EmailSetting{
					Enabled:   true,
					SmtpHost:  "smtp.example.com",
					SmtpPort:  587,
					FromEmail: "bot@example.com",
				},
			},
		},
	})
	require.NoError(t, err)

	owner, err := ts.CreateRegularUser(ctx, "removed-comment-owner")
	require.NoError(t, err)
	commenter, err := ts.CreateRegularUser(ctx, "remaining-comment-member")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)
	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "removed-owner-comments", Title: "Removed owner comments"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  commenter.ID,
		Role:    store.SpaceMemberRoleAdmin,
	}, owner.ID)
	require.NoError(t, err)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "assigned public note",
		Visibility: apipb.Visibility_PUBLIC,
		Space:      new("spaces/" + space.UID),
	}})
	require.NoError(t, err)
	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, owner.ID))

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "public comment visible to removed owner",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, sentMessage, "membership does not add a read gate to PUBLIC note endpoints")

	messageType := storepb.InboxMessage_NOTE_COMMENT
	inboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ReceiverID: &owner.ID, MessageType: &messageType})
	require.NoError(t, err)
	require.Len(t, inboxes, 1)
	response, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{Parent: "users/" + owner.Username})
	require.NoError(t, err)
	require.Len(t, response.Notifications, 1)

	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  owner.ID,
		Role:    store.SpaceMemberRoleUser,
	}, commenter.ID)
	require.NoError(t, err)
	sentMessage = nil
	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "comment visible while owner is a member",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, sentMessage)
	inboxes, err = ts.Store.ListInboxes(ctx, &store.FindInbox{ReceiverID: &owner.ID, MessageType: &messageType})
	require.NoError(t, err)
	require.Len(t, inboxes, 2)

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, commenter.ID))
	response, err = ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{Parent: "users/" + owner.Username})
	require.NoError(t, err)
	require.Len(t, response.Notifications, 2, "both PUBLIC endpoints remain readable after membership revocation")
}

func TestCreateNoteCommentSendsEmailNotificationWhenEnabled(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	var sentConfig *email.Config
	var sentMessage *email.Message
	ts.Service.NotificationEmailSender = func(config *email.Config, message *email.Message) {
		sentConfig = config
		sentMessage = message
	}

	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_NOTIFICATION,
		Value: &storepb.InstanceSetting_NotificationSetting{
			NotificationSetting: &storepb.InstanceNotificationSetting{
				Email: &storepb.InstanceNotificationSetting_EmailSetting{
					Enabled:      true,
					SmtpHost:     "smtp.example.com",
					SmtpPort:     587,
					SmtpUsername: "bot@example.com",
					SmtpPassword: "password",
					FromEmail:    "bot@example.com",
					FromName:     "Roster Tracker",
					ReplyTo:      "reply@example.com",
					UseTls:       true,
				},
			},
		},
	})
	require.NoError(t, err)

	owner, err := ts.CreateRegularUser(ctx, "email-comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "email-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note for email",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	comment, err := ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Email comment content",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	require.NotNil(t, sentConfig)
	require.Equal(t, "smtp.example.com", sentConfig.SMTPHost)
	require.Equal(t, 587, sentConfig.SMTPPort)
	require.Equal(t, "bot@example.com", sentConfig.FromEmail)
	require.True(t, sentConfig.UseTLS)

	require.NotNil(t, sentMessage)
	require.Equal(t, []string{owner.Email}, sentMessage.To)
	require.Equal(t, "reply@example.com", sentMessage.ReplyTo)
	require.Contains(t, sentMessage.Subject, "commented on your note")
	require.Contains(t, sentMessage.Body, "Hi email-comment-owner,")
	require.Contains(t, sentMessage.Body, "email-commenter commented on your note.")
	require.Contains(t, sentMessage.Body, fmt.Sprintf("http://localhost:8080/notes/%s#%s", strings.TrimPrefix(note.Name, "notes/"), strings.TrimPrefix(comment.Name, "notes/")))
	require.Contains(t, sentMessage.Body, "You are receiving this because you own this note.")
	require.NotContains(t, sentMessage.Body, "Email comment content")
	require.NotContains(t, sentMessage.Body, "Base note for email")
}

func TestCreateNoteMentionSendsEmailNotificationWhenEnabled(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	var sentMessage *email.Message
	ts.Service.NotificationEmailSender = func(_ *email.Config, message *email.Message) {
		sentMessage = message
	}

	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_NOTIFICATION,
		Value: &storepb.InstanceSetting_NotificationSetting{
			NotificationSetting: &storepb.InstanceNotificationSetting{
				Email: &storepb.InstanceNotificationSetting_EmailSetting{
					Enabled:   true,
					SmtpHost:  "smtp.example.com",
					SmtpPort:  587,
					FromEmail: "bot@example.com",
				},
			},
		},
	})
	require.NoError(t, err)

	author, err := ts.CreateRegularUser(ctx, "email-mention-author")
	require.NoError(t, err)
	authorCtx := ts.CreateUserContext(ctx, author.ID)

	target, err := ts.CreateRegularUser(ctx, "email-mention-target")
	require.NoError(t, err)

	note, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    fmt.Sprintf("Hello @%s from email", target.Username),
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	require.NotNil(t, sentMessage)
	require.Equal(t, []string{target.Email}, sentMessage.To)
	require.Contains(t, sentMessage.Subject, "mentioned you in a note")
	require.Contains(t, sentMessage.Body, "Hi email-mention-target,")
	require.Contains(t, sentMessage.Body, "email-mention-author mentioned you in a note.")
	require.Contains(t, sentMessage.Body, "http://localhost:8080/notes/"+strings.TrimPrefix(note.Name, "notes/"))
	require.Contains(t, sentMessage.Body, "You are receiving this because you were mentioned in this note.")
	require.NotContains(t, sentMessage.Body, "Hello")
	require.NotContains(t, sentMessage.Body, fmt.Sprintf("Hello @%s from email", target.Username))
}

func TestCreateNoteCommentSkipsEmailNotificationWithoutInstanceURL(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	ts.Profile.InstanceURL = ""

	var sentMessage *email.Message
	ts.Service.NotificationEmailSender = func(_ *email.Config, message *email.Message) {
		sentMessage = message
	}

	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_NOTIFICATION,
		Value: &storepb.InstanceSetting_NotificationSetting{
			NotificationSetting: &storepb.InstanceNotificationSetting{
				Email: &storepb.InstanceNotificationSetting_EmailSetting{
					Enabled:   true,
					SmtpHost:  "smtp.example.com",
					SmtpPort:  587,
					FromEmail: "bot@example.com",
				},
			},
		},
	})
	require.NoError(t, err)

	owner, err := ts.CreateRegularUser(ctx, "email-comment-no-url-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "email-comment-no-url-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note without instance URL",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Comment without instance URL",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	require.Nil(t, sentMessage)
}

func TestListUserNotificationsOmitsNotificationWhenNoteDeleted(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "notification-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Comment content",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.DeleteNote(ownerCtx, &apipb.DeleteNoteRequest{
		Name: note.Name,
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", owner.Username),
	})
	require.NoError(t, err)
	require.Empty(t, resp.Notifications, "a notification whose note subject is no longer readable must be omitted in full")
}

func TestListUserNotificationsSkipsNotificationsWithMissingUsers(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "notification-orphan")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "Comment content",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: commenter.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", owner.Username),
	})
	require.NoError(t, err)
	require.Empty(t, resp.Notifications)
}

func TestUpdateUserNotificationRequiresUpdateMask(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "notification-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "Base note", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent:  note.Name,
		Comment: &apipb.Note{Content: "Comment content", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", owner.Username),
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)

	for _, mask := range []*fieldmaskpb.FieldMask{nil, {}} {
		_, err = ts.Service.UpdateUserNotification(ownerCtx, &apipb.UpdateUserNotificationRequest{
			Notification: &apipb.UserNotification{
				Name:   resp.Notifications[0].Name,
				Status: apipb.UserNotification_ARCHIVED,
			},
			UpdateMask: mask,
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestListUserNotificationsRejectsNumericParent(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	_, err = ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: "users/1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "user not found")
}

func TestListUserNotificationsIncludesNoteMentionPayload(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	author, err := ts.CreateRegularUser(ctx, "mention-author")
	require.NoError(t, err)
	authorCtx := ts.CreateUserContext(ctx, author.ID)

	target, err := ts.CreateRegularUser(ctx, "mention-target")
	require.NoError(t, err)
	targetCtx := ts.CreateUserContext(ctx, target.ID)

	note, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    fmt.Sprintf("Hello @%s", target.Username),
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(targetCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", target.Username),
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)
	require.Equal(t, apipb.UserNotification_NOTE_MENTION, resp.Notifications[0].Type)
	require.NotNil(t, resp.Notifications[0].GetNoteMention())
	require.Equal(t, note.Name, resp.Notifications[0].GetNoteMention().Note)
	require.Empty(t, resp.Notifications[0].GetNoteMention().RelatedNote)
	require.Equal(t, author.Username, resp.Notifications[0].SenderUser.Username)
	require.Equal(t, "Hello", resp.Notifications[0].GetNoteMention().NoteSnippet)
}

func TestCreateNoteCommentMentionDoesNotDuplicateOwnerNotification(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "mention-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "mention-commenter")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Base note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    fmt.Sprintf("Hi @%s", owner.Username),
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", owner.Username),
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)
	require.Equal(t, apipb.UserNotification_NOTE_COMMENT, resp.Notifications[0].Type)
}

func TestUpdateNoteMentionOnlyNotifiesNewTargets(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	author, err := ts.CreateRegularUser(ctx, "mention-update-author")
	require.NoError(t, err)
	authorCtx := ts.CreateUserContext(ctx, author.ID)

	firstTarget, err := ts.CreateRegularUser(ctx, "mention-update-first")
	require.NoError(t, err)
	firstTargetCtx := ts.CreateUserContext(ctx, firstTarget.ID)

	secondTarget, err := ts.CreateRegularUser(ctx, "mention-update-second")
	require.NoError(t, err)
	secondTargetCtx := ts.CreateUserContext(ctx, secondTarget.ID)

	note, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	updatedNote, err := ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:       note.Name,
			Content:    fmt.Sprintf("Hello @%s", firstTarget.Username),
			Visibility: apipb.Visibility_PUBLIC,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)

	firstResp, err := ts.Service.ListUserNotifications(firstTargetCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", firstTarget.Username),
	})
	require.NoError(t, err)
	require.Len(t, firstResp.Notifications, 1)
	require.Equal(t, apipb.UserNotification_NOTE_MENTION, firstResp.Notifications[0].Type)
	require.Equal(t, updatedNote.Name, firstResp.Notifications[0].GetNoteMention().Note)

	_, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:       note.Name,
			Content:    fmt.Sprintf("Hello again @%s and @%s", firstTarget.Username, secondTarget.Username),
			Visibility: apipb.Visibility_PUBLIC,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)

	firstResp, err = ts.Service.ListUserNotifications(firstTargetCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", firstTarget.Username),
	})
	require.NoError(t, err)
	require.Len(t, firstResp.Notifications, 1)

	secondResp, err := ts.Service.ListUserNotifications(secondTargetCtx, &apipb.ListUserNotificationsRequest{
		Parent: fmt.Sprintf("users/%s", secondTarget.Username),
	})
	require.NoError(t, err)
	require.Len(t, secondResp.Notifications, 1)
	require.Equal(t, apipb.UserNotification_NOTE_MENTION, secondResp.Notifications[0].Type)
}

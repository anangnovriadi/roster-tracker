package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) ListUserNotifications(ctx context.Context, request *apipb.ListUserNotificationsRequest) (*apipb.ListUserNotificationsResponse, error) {
	user, err := s.resolveUserFromName(ctx, request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	userID := user.ID

	// Verify the requesting user has permission to view these notifications
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.ID != userID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	// Fetch inbox items from storage.
	inboxes, err := s.Store.ListInboxes(ctx, &store.FindInbox{
		ReceiverID: &userID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list inboxes: %v", err)
	}

	// Convert storage layer inboxes to API notifications.
	userIDs := make([]int32, 0, len(inboxes)*2)
	for _, inbox := range inboxes {
		userIDs = append(userIDs, inbox.ReceiverID, inbox.SenderID)
	}
	usersByID, err := s.listUsersByID(ctx, userIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification users: %v", err)
	}
	notesByID, err := s.listNotesByID(ctx, collectInboxNoteIDs(inboxes))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification notes: %v", err)
	}
	spaceContext, err := s.loadSpaceInvitationNotificationContext(ctx, currentUser, inboxes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification spaces: %v", err)
	}

	notifications := []*apipb.UserNotification{}
	for _, inbox := range inboxes {
		notification, err := s.convertInboxToUserNotificationWithUsersAndNotes(ctx, inbox, currentUser, usersByID, notesByID, spaceContext)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				slog.Warn("Skipping notification with missing user",
					slog.Int64("notification_id", int64(inbox.ID)),
					slog.Int64("receiver_id", int64(inbox.ReceiverID)),
					slog.Int64("sender_id", int64(inbox.SenderID)),
				)
				continue
			}
			return nil, status.Errorf(codes.Internal, "failed to convert inbox: %v", err)
		}
		if notification == nil || notification.Type == apipb.UserNotification_TYPE_UNSPECIFIED {
			continue
		}
		notifications = append(notifications, notification)
	}

	return &apipb.ListUserNotificationsResponse{
		Notifications: notifications,
	}, nil
}

// UpdateUserNotification updates a notification's status (e.g., marking as read/archived).
// Only the notification owner can update their notifications.
func (s *APIService) UpdateUserNotification(ctx context.Context, request *apipb.UpdateUserNotificationRequest) (*apipb.UserNotification, error) {
	if request.Notification == nil {
		return nil, status.Errorf(codes.InvalidArgument, "notification is required")
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}

	user, notificationID, err := s.resolveUserAndNotificationIDFromName(ctx, request.Notification.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid notification name: %v", err)
	}

	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}

	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.ID != user.ID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	// Verify ownership before updating
	inboxes, err := s.Store.ListInboxes(ctx, &store.FindInbox{
		ID: &notificationID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get inbox: %v", err)
	}
	if len(inboxes) == 0 {
		return nil, status.Errorf(codes.NotFound, "notification not found")
	}
	inbox := inboxes[0]
	if inbox.ReceiverID != currentUser.ID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	notesByID, err := s.listNotesByID(ctx, collectInboxNoteIDs([]*store.Inbox{inbox}))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification notes: %v", err)
	}
	spaceContext, err := s.loadSpaceInvitationNotificationContext(ctx, currentUser, []*store.Inbox{inbox})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification spaces: %v", err)
	}
	accessible, err := s.canAccessNotification(ctx, currentUser, inbox, notesByID, spaceContext)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to authorize notification: %v", err)
	}
	if !accessible {
		return nil, status.Errorf(codes.NotFound, "notification not found")
	}

	// Build update request based on field mask
	update := &store.UpdateInbox{
		ID: notificationID,
	}

	for _, path := range request.UpdateMask.Paths {
		switch path {
		case "status":
			// Convert API status enum to storage enum
			var inboxStatus store.InboxStatus
			switch request.Notification.Status {
			case apipb.UserNotification_UNREAD:
				inboxStatus = store.UNREAD
			case apipb.UserNotification_ARCHIVED:
				inboxStatus = store.ARCHIVED
			default:
				return nil, status.Errorf(codes.InvalidArgument, "invalid status")
			}
			update.Status = inboxStatus
		default:
			return nil, status.Errorf(codes.InvalidArgument, "invalid update path: %s", path)
		}
	}

	updatedInbox, err := s.Store.UpdateInbox(ctx, update)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update inbox: %v", err)
	}

	notification, err := s.convertInboxToUserNotification(ctx, updatedInbox, currentUser)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert inbox: %v", err)
	}
	if notification == nil {
		return nil, status.Errorf(codes.NotFound, "notification not found")
	}

	return notification, nil
}

// DeleteUserNotification permanently deletes a notification.
// Only the notification owner can delete their notifications.
func (s *APIService) DeleteUserNotification(ctx context.Context, request *apipb.DeleteUserNotificationRequest) (*emptypb.Empty, error) {
	user, notificationID, err := s.resolveUserAndNotificationIDFromName(ctx, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid notification name: %v", err)
	}

	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}

	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.ID != user.ID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	// Verify ownership before deletion
	inboxes, err := s.Store.ListInboxes(ctx, &store.FindInbox{
		ID: &notificationID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get inbox: %v", err)
	}
	if len(inboxes) == 0 {
		return nil, status.Errorf(codes.NotFound, "notification not found")
	}
	inbox := inboxes[0]
	if inbox.ReceiverID != currentUser.ID {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	if err := s.Store.DeleteInbox(ctx, &store.DeleteInbox{
		ID: notificationID,
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete inbox: %v", err)
	}

	return &emptypb.Empty{}, nil
}

// convertInboxToUserNotification converts a storage-layer inbox to an API notification.
// This handles the mapping between the internal inbox representation and the public API.
func (s *APIService) convertInboxToUserNotification(ctx context.Context, inbox *store.Inbox, viewer *store.User) (*apipb.UserNotification, error) {
	usersByID, err := s.listUsersByID(ctx, []int32{inbox.ReceiverID, inbox.SenderID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification users: %v", err)
	}
	notesByID, err := s.listNotesByID(ctx, collectInboxNoteIDs([]*store.Inbox{inbox}))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification notes: %v", err)
	}
	spaceContext, err := s.loadSpaceInvitationNotificationContext(ctx, viewer, []*store.Inbox{inbox})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notification spaces: %v", err)
	}
	return s.convertInboxToUserNotificationWithUsersAndNotes(ctx, inbox, viewer, usersByID, notesByID, spaceContext)
}

func collectInboxNoteIDs(inboxes []*store.Inbox) []int32 {
	noteIDs := make([]int32, 0, len(inboxes)*2)
	for _, inbox := range inboxes {
		if inbox == nil || inbox.Message == nil {
			continue
		}
		switch inbox.Message.Type {
		case storepb.InboxMessage_NOTE_COMMENT:
			payload := inbox.Message.GetNoteComment()
			if payload != nil {
				if payload.NoteId > 0 {
					noteIDs = append(noteIDs, payload.NoteId)
				}
				if payload.RelatedNoteId > 0 {
					noteIDs = append(noteIDs, payload.RelatedNoteId)
				}
			}
		case storepb.InboxMessage_NOTE_MENTION:
			payload := inbox.Message.GetNoteMention()
			if payload != nil {
				if payload.NoteId > 0 {
					noteIDs = append(noteIDs, payload.NoteId)
				}
				if payload.RelatedNoteId > 0 {
					noteIDs = append(noteIDs, payload.RelatedNoteId)
				}
			}
		default:
			// Ignore notification types without note references.
		}
	}
	return noteIDs
}

func (s *APIService) convertInboxToUserNotificationWithUsersAndNotes(ctx context.Context, inbox *store.Inbox, viewer *store.User, usersByID map[int32]*store.User, notesByID map[int32]*store.Note, spaceContext *spaceInvitationNotificationContext) (*apipb.UserNotification, error) {
	accessible, err := s.canAccessNotification(ctx, viewer, inbox, notesByID, spaceContext)
	if err != nil {
		return nil, err
	}
	if !accessible {
		return nil, nil
	}
	receiver := usersByID[inbox.ReceiverID]
	if receiver == nil {
		return nil, status.Errorf(codes.NotFound, "notification receiver not found")
	}
	sender := usersByID[inbox.SenderID]
	if sender == nil {
		return nil, status.Errorf(codes.NotFound, "notification sender not found")
	}

	notification := &apipb.UserNotification{
		Name:       fmt.Sprintf("%s/notifications/%d", BuildUserName(receiver.Username), inbox.ID),
		Sender:     BuildUserName(sender.Username),
		SenderUser: convertUserFromStore(sender, viewer),
		CreateTime: timestamppb.New(time.Unix(inbox.CreatedTs, 0)),
	}

	// Convert status from storage enum to API enum
	switch inbox.Status {
	case store.UNREAD:
		notification.Status = apipb.UserNotification_UNREAD
	case store.ARCHIVED:
		notification.Status = apipb.UserNotification_ARCHIVED
	default:
		notification.Status = apipb.UserNotification_STATUS_UNSPECIFIED
	}

	// Extract notification type and payload from the inbox message.
	if inbox.Message != nil {
		switch inbox.Message.Type {
		case storepb.InboxMessage_NOTE_COMMENT:
			notification.Type = apipb.UserNotification_NOTE_COMMENT
			payload, err := s.convertNoteCommentNotificationPayload(inbox.Message, notesByID)
			if err != nil {
				return nil, err
			}
			if payload != nil {
				notification.Payload = &apipb.UserNotification_NoteComment{
					NoteComment: payload,
				}
			}
		case storepb.InboxMessage_NOTE_MENTION:
			notification.Type = apipb.UserNotification_NOTE_MENTION
			payload, err := s.convertNoteMentionNotificationPayload(inbox.Message, notesByID)
			if err != nil {
				return nil, err
			}
			if payload != nil {
				notification.Payload = &apipb.UserNotification_NoteMention{
					NoteMention: payload,
				}
			}
		case storepb.InboxMessage_SPACE_INVITATION:
			payload := convertSpaceInvitationNotificationPayload(inbox, viewer, spaceContext)
			if payload == nil {
				// The access check already required a pending or accepted
				// invitation; a missing payload means the space disappeared
				// in between. Fail closed.
				return nil, nil
			}
			notification.Type = apipb.UserNotification_SPACE_INVITATION
			notification.Payload = &apipb.UserNotification_SpaceInvitation{
				SpaceInvitation: payload,
			}
		default:
			notification.Type = apipb.UserNotification_TYPE_UNSPECIFIED
		}
	}

	return notification, nil
}

// canAccessNotification reports whether the viewer may still read every
// subject the notification refers to. Note notifications require read access
// to each note; a space invitation requires the viewer to hold the pending
// offer or to have accepted it.
func (s *APIService) canAccessNotification(ctx context.Context, viewer *store.User, inbox *store.Inbox, notesByID map[int32]*store.Note, spaceContext *spaceInvitationNotificationContext) (bool, error) {
	if viewer == nil || viewer.RowStatus != store.Normal {
		return false, nil
	}
	if inbox == nil || inbox.Message == nil {
		return true, nil
	}
	check := func(id int32) (bool, error) {
		if id <= 0 {
			return false, nil
		}
		note := notesByID[id]
		if note == nil {
			return false, nil
		}
		readContext, err := s.buildNoteReadContextForViewer(ctx, note, viewer, false, nil)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return false, nil
			}
			return false, err
		}
		return access.CheckNoteReadContext(readContext).Allowed(), nil
	}

	switch inbox.Message.Type {
	case storepb.InboxMessage_NOTE_COMMENT:
		payload := inbox.Message.GetNoteComment()
		if payload == nil {
			return false, nil
		}
		canReadComment, err := check(payload.NoteId)
		if err != nil || !canReadComment {
			return canReadComment, err
		}
		return check(payload.RelatedNoteId)
	case storepb.InboxMessage_NOTE_MENTION:
		payload := inbox.Message.GetNoteMention()
		if payload == nil {
			return false, nil
		}
		canReadNote, err := check(payload.NoteId)
		if err != nil || !canReadNote || payload.RelatedNoteId <= 0 {
			return canReadNote, err
		}
		return check(payload.RelatedNoteId)
	case storepb.InboxMessage_SPACE_INVITATION:
		space, _, state := spaceContext.resolve(inbox.Message.GetSpaceInvitation().GetSpaceId())
		return space != nil && state != apipb.UserNotification_SpaceInvitationPayload_STATE_UNSPECIFIED, nil
	default:
		return true, nil
	}
}

func (s *APIService) noteNotificationSnippet(note *store.Note) (string, error) {
	if note == nil || note.Content == "" {
		return "", nil
	}

	snippet, err := s.getNoteContentSnippet(note.Content)
	if err != nil {
		return "", err
	}
	return snippet, nil
}

func (s *APIService) convertNoteCommentNotificationPayload(message *storepb.InboxMessage, notesByID map[int32]*store.Note) (*apipb.UserNotification_NoteCommentPayload, error) {
	noteComment := message.GetNoteComment()
	if message == nil || message.Type != storepb.InboxMessage_NOTE_COMMENT || noteComment == nil {
		return nil, nil
	}

	commentNote := notesByID[noteComment.NoteId]
	relatedNote := notesByID[noteComment.RelatedNoteId]

	noteSnippet, err := s.noteNotificationSnippet(commentNote)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get comment note snippet")
	}
	relatedNoteSnippet, err := s.noteNotificationSnippet(relatedNote)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get related note snippet")
	}

	return &apipb.UserNotification_NoteCommentPayload{
		Note:               fmt.Sprintf("%s%s", NoteNamePrefix, commentNote.UID),
		RelatedNote:        fmt.Sprintf("%s%s", NoteNamePrefix, relatedNote.UID),
		NoteSnippet:        noteSnippet,
		RelatedNoteSnippet: relatedNoteSnippet,
	}, nil
}

func (s *APIService) convertNoteMentionNotificationPayload(message *storepb.InboxMessage, notesByID map[int32]*store.Note) (*apipb.UserNotification_NoteMentionPayload, error) {
	noteMention := message.GetNoteMention()
	if message == nil || message.Type != storepb.InboxMessage_NOTE_MENTION || noteMention == nil {
		return nil, nil
	}

	note := notesByID[noteMention.NoteId]

	noteSnippet, err := s.noteNotificationSnippet(note)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mention note snippet")
	}

	payload := &apipb.UserNotification_NoteMentionPayload{
		Note:        fmt.Sprintf("%s%s", NoteNamePrefix, note.UID),
		NoteSnippet: noteSnippet,
	}
	if noteMention.RelatedNoteId != 0 {
		relatedNote := notesByID[noteMention.RelatedNoteId]
		if relatedNote != nil {
			payload.RelatedNote = fmt.Sprintf("%s%s", NoteNamePrefix, relatedNote.UID)
			relatedNoteSnippet, err := s.noteNotificationSnippet(relatedNote)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get related note snippet")
			}
			payload.RelatedNoteSnippet = relatedNoteSnippet
		}
	}

	return payload, nil
}

package notification

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/internal/email"
	"github.com/anangnovriadi/roster-tracker/internal/profile"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// EmailSender sends a prepared email message with the given SMTP configuration.
type EmailSender func(*email.Config, *email.Message)

// EmailDispatcher dispatches notification emails for inbox events.
type EmailDispatcher struct {
	profile *profile.Profile
	store   *store.Store
	sender  EmailSender
}

// NewEmailDispatcher creates a notification email dispatcher.
func NewEmailDispatcher(profile *profile.Profile, store *store.Store, sender EmailSender) *EmailDispatcher {
	if sender == nil {
		sender = email.SendAsync
	}
	return &EmailDispatcher{
		profile: profile,
		store:   store,
		sender:  sender,
	}
}

// DispatchInboxEmail sends the email notification for an inbox entry when configured.
func (d *EmailDispatcher) DispatchInboxEmail(ctx context.Context, inbox *store.Inbox) error {
	if inbox == nil || inbox.Message == nil {
		return nil
	}

	setting, err := d.store.GetInstanceNotificationSetting(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get notification setting")
	}
	emailSetting := setting.GetEmail()
	if emailSetting == nil || !emailSetting.Enabled {
		return nil
	}
	if d.baseURL() == "" {
		slog.Warn("Skipping inbox email notification because instance URL is required",
			slog.Int64("inbox_id", int64(inbox.ID)),
			slog.Int64("receiver_id", int64(inbox.ReceiverID)))
		return nil
	}

	receiver, err := d.store.GetUser(ctx, &store.FindUser{ID: &inbox.ReceiverID})
	if err != nil {
		return errors.Wrap(err, "failed to get notification receiver")
	}
	if receiver == nil || strings.TrimSpace(receiver.Email) == "" {
		return nil
	}

	sender, err := d.store.GetUser(ctx, &store.FindUser{ID: &inbox.SenderID})
	if err != nil {
		return errors.Wrap(err, "failed to get notification sender")
	}
	if sender == nil {
		return nil
	}

	notesByID, err := d.listNotesByID(ctx, collectInboxNoteIDs([]*store.Inbox{inbox}), receiver.ID)
	if err != nil {
		return errors.Wrap(err, "failed to get notification notes")
	}

	message, err := d.buildInboxEmailMessage(ctx, inbox, receiver, sender, notesByID)
	if err != nil {
		return err
	}
	if message == nil {
		return nil
	}
	message.ReplyTo = emailSetting.ReplyTo

	config := EmailConfigFromInstanceSetting(emailSetting)
	if err := config.Validate(); err != nil {
		return errors.Wrap(err, "invalid notification email setting")
	}

	d.sender(config, message)
	return nil
}

// EmailConfigFromInstanceSetting converts persisted notification settings into SMTP config.
func EmailConfigFromInstanceSetting(setting *storepb.InstanceNotificationSetting_EmailSetting) *email.Config {
	if setting == nil {
		return &email.Config{}
	}
	return &email.Config{
		SMTPHost:     setting.SmtpHost,
		SMTPPort:     int(setting.SmtpPort),
		SMTPUsername: setting.SmtpUsername,
		SMTPPassword: setting.SmtpPassword,
		FromEmail:    setting.FromEmail,
		FromName:     setting.FromName,
		UseTLS:       setting.UseTls,
		UseSSL:       setting.UseSsl,
	}
}

// NewTestEmailMessage builds the plain-text test email for notification settings.
func NewTestEmailMessage(recipientEmail, replyTo string) *email.Message {
	return &email.Message{
		To:      []string{recipientEmail},
		Subject: "[Roster Tracker] Test email",
		Body:    "This is a test email from your Roster Tracker notification settings.",
		ReplyTo: replyTo,
	}
}

// ValidateEmailSetting validates notification email SMTP settings.
func ValidateEmailSetting(setting *storepb.InstanceNotificationSetting_EmailSetting) error {
	return EmailConfigFromInstanceSetting(setting).Validate()
}

// SendTestEmail sends a plain-text test email using notification email settings.
func SendTestEmail(setting *storepb.InstanceNotificationSetting_EmailSetting, recipientEmail string) error {
	return email.Send(EmailConfigFromInstanceSetting(setting), NewTestEmailMessage(recipientEmail, setting.GetReplyTo()))
}

func (d *EmailDispatcher) buildInboxEmailMessage(ctx context.Context, inbox *store.Inbox, receiver *store.User, sender *store.User, notesByID map[int32]*store.Note) (*email.Message, error) {
	senderName := displayNameForEmail(sender)
	switch inbox.Message.Type {
	case storepb.InboxMessage_NOTE_COMMENT:
		return d.buildNoteCommentEmailMessage(inbox.Message, receiver, senderName, notesByID)
	case storepb.InboxMessage_NOTE_MENTION:
		return d.buildNoteMentionEmailMessage(inbox.Message, receiver, senderName, notesByID)
	case storepb.InboxMessage_SPACE_INVITATION:
		return d.buildSpaceInvitationEmailMessage(ctx, inbox.Message, receiver, senderName)
	default:
		return nil, nil
	}
}

// buildSpaceInvitationEmailMessage renders the invitation email only while
// the receiver still holds the pending offer, so a revoked or already handled
// invitation never reaches their mailbox.
func (d *EmailDispatcher) buildSpaceInvitationEmailMessage(ctx context.Context, message *storepb.InboxMessage, receiver *store.User, senderName string) (*email.Message, error) {
	payload := message.GetSpaceInvitation()
	if payload == nil || payload.SpaceId <= 0 {
		return nil, nil
	}
	space, err := d.store.GetSpace(ctx, &store.FindSpace{ID: &payload.SpaceId})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get invitation space")
	}
	if space == nil {
		return nil, nil
	}
	invitation, err := d.store.GetSpaceInvitation(ctx, &store.FindSpaceInvitation{
		SpaceID:      &space.ID,
		UserID:       &receiver.ID,
		ViewerUserID: &receiver.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get space invitation")
	}
	if invitation == nil {
		return nil, nil
	}
	url := d.inboxURL()
	if url == "" {
		return nil, nil
	}

	roleName := "a member"
	if invitation.Role == store.SpaceMemberRoleAdmin {
		roleName = "an administrator"
	}
	body := []string{
		fmt.Sprintf("Hi %s,", displayNameForEmail(receiver)),
		"",
		fmt.Sprintf("%s invited you to join the Space \"%s\" as %s.", senderName, space.Title, roleName),
		"",
		"Review the invitation in Roster Tracker:",
		url,
		"",
		"You are receiving this because an administrator of this Space invited you.",
	}

	return &email.Message{
		To:      []string{receiver.Email},
		Subject: fmt.Sprintf("[Roster Tracker] %s invited you to join \"%s\"", senderName, space.Title),
		Body:    strings.Join(body, "\n"),
	}, nil
}

func (d *EmailDispatcher) buildNoteCommentEmailMessage(message *storepb.InboxMessage, receiver *store.User, senderName string, notesByID map[int32]*store.Note) (*email.Message, error) {
	payload := message.GetNoteComment()
	if payload == nil {
		return nil, nil
	}
	commentNote := notesByID[payload.NoteId]
	relatedNote := notesByID[payload.RelatedNoteId]
	if !canViewerAccessNote(receiver, commentNote) || !canViewerAccessNote(receiver, relatedNote) {
		return nil, nil
	}
	url := d.noteCommentURL(relatedNote, commentNote)
	if url == "" {
		return nil, nil
	}

	body := []string{
		fmt.Sprintf("Hi %s,", displayNameForEmail(receiver)),
		"",
		fmt.Sprintf("%s commented on your note.", senderName),
		"",
		"Open in Roster Tracker:",
		url,
		"",
		"You are receiving this because you own this note.",
	}

	return &email.Message{
		To:      []string{receiver.Email},
		Subject: fmt.Sprintf("[Roster Tracker] %s commented on your note", senderName),
		Body:    strings.Join(body, "\n"),
	}, nil
}

func (d *EmailDispatcher) buildNoteMentionEmailMessage(message *storepb.InboxMessage, receiver *store.User, senderName string, notesByID map[int32]*store.Note) (*email.Message, error) {
	payload := message.GetNoteMention()
	if payload == nil {
		return nil, nil
	}
	note := notesByID[payload.NoteId]
	if !canViewerAccessNote(receiver, note) {
		return nil, nil
	}
	// A mention created from a comment carries its conversational context in
	// RelatedNoteId. The notification represents that relation, so suppress the
	// complete email unless the receiver can still read both endpoints.
	if payload.RelatedNoteId > 0 {
		if !canViewerAccessNote(receiver, notesByID[payload.RelatedNoteId]) {
			return nil, nil
		}
	}
	url := d.noteURL(note)
	if url == "" {
		return nil, nil
	}

	body := []string{
		fmt.Sprintf("Hi %s,", displayNameForEmail(receiver)),
		"",
		fmt.Sprintf("%s mentioned you in a note.", senderName),
		"",
		"Open in Roster Tracker:",
		url,
		"",
		"You are receiving this because you were mentioned in this note.",
	}

	return &email.Message{
		To:      []string{receiver.Email},
		Subject: fmt.Sprintf("[Roster Tracker] %s mentioned you in a note", senderName),
		Body:    strings.Join(body, "\n"),
	}, nil
}

func (d *EmailDispatcher) listNotesByID(ctx context.Context, noteIDs []int32, viewerID int32) (map[int32]*store.Note, error) {
	if len(noteIDs) == 0 {
		return map[int32]*store.Note{}, nil
	}

	uniqueNoteIDs := make([]int32, 0, len(noteIDs))
	seenNoteIDs := make(map[int32]struct{}, len(noteIDs))
	for _, noteID := range noteIDs {
		if noteID == 0 {
			continue
		}
		if _, seen := seenNoteIDs[noteID]; seen {
			continue
		}
		seenNoteIDs[noteID] = struct{}{}
		uniqueNoteIDs = append(uniqueNoteIDs, noteID)
	}
	if len(uniqueNoteIDs) == 0 {
		return map[int32]*store.Note{}, nil
	}

	notes, err := d.store.ListNotes(ctx, &store.FindNote{
		IDList: uniqueNoteIDs,
		Access: &store.NoteAccessScope{UserID: &viewerID, AllowPublic: true, AllowProtected: true},
	})
	if err != nil {
		return nil, err
	}

	notesByID := make(map[int32]*store.Note, len(notes))
	for _, note := range notes {
		notesByID[note.ID] = note
	}
	return notesByID, nil
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
				noteIDs = append(noteIDs, payload.NoteId, payload.RelatedNoteId)
			}
		case storepb.InboxMessage_NOTE_MENTION:
			payload := inbox.Message.GetNoteMention()
			if payload != nil {
				noteIDs = append(noteIDs, payload.NoteId, payload.RelatedNoteId)
			}
		default:
			// Ignore notification types without note references.
		}
	}
	return noteIDs
}

func displayNameForEmail(user *store.User) string {
	if user == nil {
		return "there"
	}
	if strings.TrimSpace(user.Nickname) != "" {
		return user.Nickname
	}
	if strings.TrimSpace(user.Username) != "" {
		return user.Username
	}
	return "there"
}

func (d *EmailDispatcher) baseURL() string {
	if d.profile == nil || strings.TrimSpace(d.profile.InstanceURL) == "" {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(d.profile.InstanceURL), "/")
}

func (d *EmailDispatcher) inboxURL() string {
	baseURL := d.baseURL()
	if baseURL == "" {
		return ""
	}
	return baseURL + "/inbox"
}

func (d *EmailDispatcher) noteURL(note *store.Note) string {
	baseURL := d.baseURL()
	if note == nil || note.UID == "" || baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/notes/%s", baseURL, note.UID)
}

func (d *EmailDispatcher) noteCommentURL(relatedNote *store.Note, commentNote *store.Note) string {
	baseURL := d.baseURL()
	if relatedNote == nil || relatedNote.UID == "" || commentNote == nil || commentNote.UID == "" || baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/notes/%s#%s", baseURL, relatedNote.UID, commentNote.UID)
}

func canViewerAccessNote(viewer *store.User, note *store.Note) bool {
	if note == nil {
		return false
	}
	if note.Visibility == store.Private {
		return viewer != nil && viewer.ID == note.CreatorID
	}
	return true
}

package api

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/internal/webhook"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/server/auth"
	"github.com/anangnovriadi/roster-tracker/store"
)

// DispatchNoteCreatedWebhook dispatches a webhook when a note is created.
func (s *APIService) DispatchNoteCreatedWebhook(ctx context.Context, note *apipb.Note) error {
	return s.dispatchNoteRelatedWebhook(ctx, note, "notes.note.created")
}

// DispatchNoteUpdatedWebhook dispatches webhook when note is updated.
func (s *APIService) DispatchNoteUpdatedWebhook(ctx context.Context, note *apipb.Note) error {
	return s.dispatchNoteRelatedWebhook(ctx, note, "notes.note.updated")
}

// DispatchNoteDeletedWebhook dispatches webhook when note is deleted.
func (s *APIService) DispatchNoteDeletedWebhook(ctx context.Context, note *apipb.Note) error {
	return s.dispatchNoteRelatedWebhook(ctx, note, "notes.note.deleted")
}

// DispatchNoteCommentCreatedWebhook dispatches webhook to the related note owner when a comment is created.
func (s *APIService) DispatchNoteCommentCreatedWebhook(
	ctx context.Context,
	comment, relatedNote *store.Note,
	relatedNoteCreatorID int32,
) error {
	receiver, err := s.Store.GetUser(ctx, &store.FindUser{ID: &relatedNoteCreatorID})
	if err != nil {
		return err
	}
	if receiver == nil || receiver.RowStatus != store.Normal {
		return nil
	}
	for _, subject := range []*store.Note{comment, relatedNote} {
		readContext, err := s.buildNoteReadContextForViewer(ctx, subject, receiver, false, nil)
		if err != nil {
			return errors.Wrap(err, "failed to resolve webhook subject access")
		}
		decision := access.CheckNoteReadContext(readContext)
		if !decision.Allowed() {
			return nil
		}
	}
	// The request context belongs to the comment author, but the webhook belongs
	// to the context note's author. Build the prepared payload under the receiver's
	// identity before it enters the existing asynchronous delivery queue.
	receiverCtx := auth.SetUserInContext(ctx, receiver, "")
	reactions, err := s.Store.ListReactions(receiverCtx, &store.FindReaction{NoteID: &comment.ID})
	if err != nil {
		return err
	}
	attachments, err := s.Store.ListAttachments(receiverCtx, &store.FindAttachment{NoteID: &comment.ID})
	if err != nil {
		return err
	}
	relations, err := s.loadNoteRelations(receiverCtx, comment)
	if err != nil {
		return err
	}
	commentMessage, err := s.convertNoteFromStore(receiverCtx, comment, reactions, attachments, relations)
	if err != nil {
		return err
	}
	webhooks, err := s.Store.GetUserWebhooks(ctx, relatedNoteCreatorID)
	if err != nil {
		return err
	}
	for _, hook := range webhooks {
		webhook.PostAsync(newNoteWebhookRequest(hook, "notes.note.comment.created", commentMessage))
	}
	return nil
}

func (s *APIService) dispatchNoteRelatedWebhook(ctx context.Context, note *apipb.Note, activityType string) error {
	creator, err := ResolveUserByName(ctx, s.Store, note.Creator)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid note creator")
	}
	if creator == nil {
		return status.Errorf(codes.NotFound, "note creator not found")
	}
	creatorID := creator.ID
	webhooks, err := s.Store.GetUserWebhooks(ctx, creatorID)
	if err != nil {
		return err
	}
	for _, hook := range webhooks {
		webhook.PostAsync(newNoteWebhookRequest(hook, activityType, note))
	}
	return nil
}

// noteWebhookPayload is the JSON body delivered to user webhooks for note
// activity. Field names are part of the webhook contract.
type noteWebhookPayload struct {
	// URL is the destination the payload was sent to.
	URL string `json:"url"`
	// ActivityType names the event, for example "notes.note.created".
	ActivityType string `json:"activityType"`
	// Creator is the resource name of the note creator. Format: users/{user}
	Creator string `json:"creator"`
	// Note is the note that triggered the event.
	Note *apipb.Note `json:"note"`
}

func newNoteWebhookRequest(hook *storepb.WebhooksUserSetting_Webhook, activityType string, note *apipb.Note) *webhook.Request {
	return &webhook.Request{
		URL:           hook.Url,
		Label:         activityType,
		SigningSecret: hook.SigningSecret,
		Payload: &noteWebhookPayload{
			URL:          hook.Url,
			ActivityType: activityType,
			Creator:      note.Creator,
			Note:         note,
		},
	}
}

func (s *APIService) getNoteContentSnippet(content string) (string, error) {
	// Use goldmark service for snippet generation
	snippet, err := s.MarkdownService.GenerateSnippet([]byte(content), 64)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate snippet")
	}
	return snippet, nil
}

// parseNoteOrderBy parses the order_by field and sets the appropriate ordering in noteFind.
// Follows AIP-132: supports comma-separated list of fields with optional "desc" suffix.
// Example: "pinned desc, create_time desc" or "update_time asc".

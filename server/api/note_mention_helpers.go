package api

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/core/access"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) resolveMentionTargets(ctx context.Context, content string) (map[int32]*store.User, error) {
	targets := make(map[int32]*store.User)
	if content == "" {
		return targets, nil
	}

	data, err := s.MarkdownService.ExtractAll([]byte(content))
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract mentions")
	}
	if len(data.Mentions) == 0 {
		return targets, nil
	}

	normal := store.Normal
	users, err := s.Store.ListUsers(ctx, &store.FindUser{
		UsernameList: data.Mentions,
		RowStatus:    &normal,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve mention users")
	}

	for _, user := range users {
		targets[user.ID] = user
	}

	return targets, nil
}

// mentionContextFacts holds the viewer-independent authorization facts for a
// mention's note and its optional context note. They are resolved once per
// dispatch and reused for every mentioned user.
type mentionContextFacts struct {
	note    access.NoteReadFacts
	related *access.NoteReadFacts
}

func (s *APIService) resolveMentionContextFacts(ctx context.Context, note *store.Note, relatedNote *store.Note) (*mentionContextFacts, error) {
	noteFacts, err := access.ResolveNoteReadFacts(ctx, s.Store, note)
	if err != nil {
		return nil, err
	}
	facts := &mentionContextFacts{note: noteFacts}
	if relatedNote != nil {
		relatedFacts, err := access.ResolveNoteReadFacts(ctx, s.Store, relatedNote)
		if err != nil {
			return nil, err
		}
		facts.related = &relatedFacts
	}
	return facts, nil
}

func (s *APIService) canUserAccessMentionContext(ctx context.Context, facts *mentionContextFacts, target *store.User) bool {
	if target == nil || facts == nil || facts.note.Note == nil {
		return false
	}
	readContext, err := facts.note.WithViewer(ctx, s.Store, target, false, nil)
	if err != nil || !access.CheckNoteReadContext(readContext).Allowed() {
		return false
	}
	if facts.related == nil {
		return true
	}
	relatedContext, err := facts.related.WithViewer(ctx, s.Store, target, false, nil)
	if err != nil {
		return false
	}
	return access.CheckNoteReadContext(relatedContext).Allowed()
}

// canUserAccessMentionNotes is the one-shot form for a single target, where
// there is no fan-out to amortize the fact resolution over.
func (s *APIService) canUserAccessMentionNotes(ctx context.Context, target *store.User, note *store.Note, relatedNote *store.Note) bool {
	facts, err := s.resolveMentionContextFacts(ctx, note, relatedNote)
	if err != nil {
		return false
	}
	return s.canUserAccessMentionContext(ctx, facts, target)
}

func (s *APIService) shouldSkipMentionInbox(ctx context.Context, facts *mentionContextFacts, target *store.User, note *store.Note, relatedNote *store.Note) bool {
	if target == nil || note == nil {
		return true
	}

	if target.ID == note.CreatorID {
		return true
	}

	// Comment creation already generates a note-comment inbox item for the parent creator.
	if relatedNote != nil && target.ID == relatedNote.CreatorID && note.CreatorID != relatedNote.CreatorID {
		return true
	}

	return !s.canUserAccessMentionContext(ctx, facts, target)
}

func (s *APIService) dispatchNoteMentionNotifications(ctx context.Context, note *store.Note, relatedNote *store.Note, previousContent string) error {
	if note == nil {
		return nil
	}

	currentTargets, err := s.resolveMentionTargets(ctx, note.Content)
	if err != nil {
		return err
	}
	if len(currentTargets) == 0 {
		return nil
	}

	previousTargets, err := s.resolveMentionTargets(ctx, previousContent)
	if err != nil {
		return err
	}

	facts, err := s.resolveMentionContextFacts(ctx, note, relatedNote)
	if err != nil {
		return errors.Wrap(err, "failed to resolve mention context")
	}

	for userID, target := range currentTargets {
		if _, exists := previousTargets[userID]; exists {
			continue
		}
		if s.shouldSkipMentionInbox(ctx, facts, target, note, relatedNote) {
			continue
		}

		payload := &storepb.InboxMessage_NoteMentionPayload{
			NoteId: note.ID,
		}
		if relatedNote != nil {
			payload.RelatedNoteId = relatedNote.ID
		}

		if _, err := s.createInboxWithEmailNotification(ctx, &store.Inbox{
			SenderID:   note.CreatorID,
			ReceiverID: target.ID,
			Status:     store.UNREAD,
			Message: &storepb.InboxMessage{
				Type: storepb.InboxMessage_NOTE_MENTION,
				Payload: &storepb.InboxMessage_NoteMention{
					NoteMention: payload,
				},
			},
		}); err != nil {
			return errors.Wrap(err, "failed to create mention inbox")
		}
	}

	return nil
}

func (s *APIService) dispatchNoteMentionNotificationsBestEffort(ctx context.Context, note *store.Note, relatedNote *store.Note, previousContent string) {
	if err := s.dispatchNoteMentionNotifications(ctx, note, relatedNote, previousContent); err != nil {
		slog.Warn("Failed to dispatch note mention notifications", slog.Any("err", err), slog.Int64("note_id", int64(note.ID)))
	}
}

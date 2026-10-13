package api

import (
	"context"
	"io"
	"maps"
	"slices"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/anangnovriadi/roster-tracker/core/noteexport"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// archiveQueryChunk bounds the number of IDs placed in one IN clause.
const archiveQueryChunk = 500

// noteExportData gathers everything one user's archive needs before any
// entry is written, so the manifest can carry accurate counts.
type noteExportData struct {
	notes            []*store.Note
	attachmentsByID  map[int32][]*store.Attachment
	relatedIDsByID   map[int32][]int32
	reactionsByID    map[int32][]*store.Reaction
	spacesByID       map[int32]*store.Space
	noteUIDs         map[int32]string
	usernames        map[int32]string
	attachmentsCount int
}

// WriteNoteExport writes a Roster Tracker export file holding every note the user
// created, including comments and archived notes, and the attachments linked
// to them. Nothing another user created is included.
func (s *APIService) WriteNoteExport(ctx context.Context, user *store.User, w io.Writer) error {
	if user == nil {
		return errors.New("user is required")
	}
	export, err := s.loadNoteExportData(ctx, user)
	if err != nil {
		return err
	}

	exportTime := time.Now().UTC().Truncate(time.Second)
	writer := noteexport.NewWriter(w, exportTime)
	if err := writer.WriteManifest(&noteexport.Manifest{
		Generator:  noteexport.Generator{Name: "roster-tracker", Version: s.Profile.Version},
		ExportTime: noteexport.FormatTime(exportTime.Unix()),
		Scope: noteexport.Scope{
			Kind: noteexport.ScopeKindUser,
			User: &noteexport.ScopeUser{Username: user.Username, DisplayName: user.Nickname},
		},
		Counts: &noteexport.Counts{Notes: len(export.notes), Attachments: export.attachmentsCount},
	}); err != nil {
		return err
	}

	for _, note := range export.notes {
		record, err := s.buildNoteExportRecord(ctx, writer, user, export, note)
		if err != nil {
			return err
		}
		if err := writer.WriteNote(record, []byte(note.Content)); err != nil {
			return err
		}
	}
	return writer.Close()
}

func (s *APIService) loadNoteExportData(ctx context.Context, user *store.User) (*noteExportData, error) {
	notes, err := s.Store.ListNotes(ctx, &store.FindNote{CreatorID: &user.ID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list notes")
	}
	export := &noteExportData{
		notes:           notes,
		attachmentsByID: make(map[int32][]*store.Attachment),
		relatedIDsByID:  make(map[int32][]int32),
		reactionsByID:   make(map[int32][]*store.Reaction),
		spacesByID:      make(map[int32]*store.Space),
		noteUIDs:        make(map[int32]string),
		usernames:       map[int32]string{user.ID: user.Username},
	}
	noteIDs := make([]int32, 0, len(notes))
	for _, note := range notes {
		noteIDs = append(noteIDs, note.ID)
		export.noteUIDs[note.ID] = note.UID
		if note.SpaceID != nil {
			if _, ok := export.spacesByID[*note.SpaceID]; !ok {
				space, err := s.Store.GetSpace(ctx, &store.FindSpace{ID: note.SpaceID})
				if err != nil {
					return nil, errors.Wrap(err, "failed to get space")
				}
				export.spacesByID[*note.SpaceID] = space
			}
		}
	}

	foreignNoteIDs := make(map[int32]struct{})
	reactorIDs := make(map[int32]struct{})
	relationType := store.NoteRelationReference
	for chunk := range slices.Chunk(noteIDs, archiveQueryChunk) {
		attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{NoteIDList: chunk})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list attachments")
		}
		for _, attachment := range attachments {
			if attachment.NoteID == nil {
				continue
			}
			export.attachmentsByID[*attachment.NoteID] = append(export.attachmentsByID[*attachment.NoteID], attachment)
			if attachment.StorageType != storepb.AttachmentStorageType_EXTERNAL {
				export.attachmentsCount++
			}
		}

		relations, err := s.Store.ListNoteRelations(ctx, &store.FindNoteRelation{NoteIDList: chunk, Type: &relationType})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list note relations")
		}
		for _, relation := range relations {
			// NoteIDList matches either endpoint; only outgoing relations from
			// the user's own notes belong to their records.
			if _, own := export.noteUIDs[relation.NoteID]; !own {
				continue
			}
			export.relatedIDsByID[relation.NoteID] = append(export.relatedIDsByID[relation.NoteID], relation.RelatedNoteID)
			if _, own := export.noteUIDs[relation.RelatedNoteID]; !own {
				foreignNoteIDs[relation.RelatedNoteID] = struct{}{}
			}
		}

		reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{NoteIDList: chunk})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list reactions")
		}
		for _, reaction := range reactions {
			export.reactionsByID[reaction.NoteID] = append(export.reactionsByID[reaction.NoteID], reaction)
			if reaction.CreatorID != user.ID {
				reactorIDs[reaction.CreatorID] = struct{}{}
			}
		}
	}

	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(foreignNoteIDs)), archiveQueryChunk) {
		related, err := s.Store.ListNotes(ctx, &store.FindNote{IDList: chunk, ExcludeContent: true})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list related notes")
		}
		for _, note := range related {
			export.noteUIDs[note.ID] = note.UID
		}
	}
	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(reactorIDs)), archiveQueryChunk) {
		users, err := s.Store.ListUsers(ctx, &store.FindUser{IDList: chunk})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list users")
		}
		for _, reactor := range users {
			export.usernames[reactor.ID] = reactor.Username
		}
	}
	return export, nil
}

func (s *APIService) buildNoteExportRecord(ctx context.Context, writer *noteexport.Writer, user *store.User, export *noteExportData, note *store.Note) (*noteexport.Note, error) {
	record := &noteexport.Note{
		UID:        note.UID,
		Creator:    user.Username,
		CreateTime: noteexport.FormatTime(note.CreatedTs),
		UpdateTime: noteexport.FormatTime(note.UpdatedTs),
		State:      string(note.RowStatus),
		Visibility: note.Visibility.String(),
		Pinned:     note.Pinned,
		Tags:       note.Payload.GetTags(),
	}
	if location := note.Payload.GetLocation(); location != nil {
		record.Location = &noteexport.Location{
			Placeholder: location.Placeholder,
			Latitude:    location.Latitude,
			Longitude:   location.Longitude,
		}
	}
	if note.SpaceID != nil {
		if space := export.spacesByID[*note.SpaceID]; space != nil {
			record.Space = &noteexport.Space{UID: space.UID, Title: space.Title}
		}
	}
	if note.ParentUID != nil && *note.ParentUID != "" {
		record.Parent = *note.ParentUID
	}
	for _, relatedID := range export.relatedIDsByID[note.ID] {
		uid, ok := export.noteUIDs[relatedID]
		if !ok {
			continue
		}
		record.Relations = append(record.Relations, noteexport.Relation{Type: noteexport.RelationReference, Note: uid})
	}
	for _, reaction := range export.reactionsByID[note.ID] {
		username, ok := export.usernames[reaction.CreatorID]
		if !ok {
			continue
		}
		record.Reactions = append(record.Reactions, noteexport.Reaction{
			ReactionType: reaction.ReactionType,
			Creator:      username,
			CreateTime:   noteexport.FormatTime(reaction.CreatedTs),
		})
	}
	for _, attachment := range export.attachmentsByID[note.ID] {
		entry := noteexport.Attachment{
			UID:        attachment.UID,
			Filename:   attachment.Filename,
			Type:       attachment.Type,
			Size:       attachment.Size,
			CreateTime: noteexport.FormatTime(attachment.CreatedTs),
		}
		if metadata := convertMediaMetadataFromStore(attachment.Payload.GetMediaMetadata()); metadata != nil {
			raw, err := protojson.Marshal(metadata)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to encode media metadata of attachment %s", attachment.UID)
			}
			entry.MediaMetadata = raw
		}
		if attachment.StorageType == storepb.AttachmentStorageType_EXTERNAL {
			entry.ExternalLink = attachment.Reference
		} else {
			content, err := s.openAttachmentContent(ctx, attachment)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to read attachment %s", attachment.UID)
			}
			entry.Path = noteexport.AttachmentPath(attachment.UID, attachment.Filename)
			digest, size, err := writer.WriteAttachment(entry.Path, content)
			_ = content.Close()
			if err != nil {
				return nil, err
			}
			entry.SHA256 = digest
			entry.Size = size
		}
		record.Attachments = append(record.Attachments, entry)
	}
	return record, nil
}

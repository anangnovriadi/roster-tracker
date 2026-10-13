package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/anangnovriadi/roster-tracker/core/noteexport"
	"github.com/anangnovriadi/roster-tracker/core/notepayload"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// PlanNoteImport classifies every record of an archive against the
// instance without writing anything.
func (s *APIService) PlanNoteImport(ctx context.Context, user *store.User, archive *noteexport.File) (*apipb.ImportNotesPlan, error) {
	if user == nil || archive == nil {
		return nil, errors.New("user and archive are required")
	}
	exportTime, err := noteexport.ParseTime(archive.Manifest.ExportTime)
	if err != nil {
		return nil, errors.Wrap(err, "invalid export time")
	}
	plan := &apipb.ImportNotesPlan{
		ExportTime: timestamppb.New(time.Unix(exportTime, 0)),
		Generator:  archive.Manifest.Generator.Name + " " + archive.Manifest.Generator.Version,
		NoteCount:  int32(len(archive.Notes)),
		Warnings:   archiveIssues(archive.Warnings),
	}
	if archive.Manifest.Scope.User != nil {
		plan.Exporter = archive.Manifest.Scope.User.Username
	}
	for _, record := range archive.Notes {
		for _, attachment := range record.Attachments {
			if attachment.Path != "" {
				plan.AttachmentCount++
			}
		}
	}
	existing, err := s.findNotesByUID(ctx, archive, true)
	if err != nil {
		return nil, err
	}
	for _, record := range archive.Notes {
		switch note := existing[record.UID]; {
		case note == nil:
			plan.New++
		case note.CreatorID == user.ID:
			plan.Existing++
		default:
			plan.Renamed++
		}
	}
	return plan, nil
}

// findNotesByUID loads the instance notes whose UIDs appear in the archive.
func (s *APIService) findNotesByUID(ctx context.Context, archive *noteexport.File, excludeContent bool) (map[string]*store.Note, error) {
	uids := make([]string, 0, len(archive.Notes))
	for _, record := range archive.Notes {
		uids = append(uids, record.UID)
	}
	found := make(map[string]*store.Note, len(uids))
	for chunk := range slices.Chunk(uids, archiveQueryChunk) {
		notes, err := s.Store.ListNotes(ctx, &store.FindNote{UIDList: chunk, ExcludeContent: excludeContent})
		if err != nil {
			return nil, errors.Wrap(err, "failed to look up notes")
		}
		for _, note := range notes {
			found[note.UID] = note
		}
	}
	return found, nil
}

func archiveIssues(warnings []noteexport.Warning) []*apipb.ImportNotesIssue {
	issues := make([]*apipb.ImportNotesIssue, 0, len(warnings))
	for _, warning := range warnings {
		issues = append(issues, &apipb.ImportNotesIssue{NoteUid: warning.UID, Message: warning.Message})
	}
	return issues
}

// writtenNote pairs an archive record with the instance note it was created
// as or merged into, for the relation pass.
type writtenNote struct {
	record *noteexport.Note
	note   *store.Note
}

type noteImporter struct {
	service *APIService
	ctx     context.Context
	user    *store.User
	archive *noteexport.File
	policy  apipb.ImportNotesRequest_ConflictPolicy
	report  *apipb.ImportNotesReport

	storageSetting     *storepb.InstanceStorageSetting
	contentLengthLimit int
	// existing holds the instance notes whose UIDs the archive uses, keyed by
	// UID, loaded once before any write.
	existing map[string]*store.Note
	// resolved maps archive note UIDs to the instance notes they became,
	// including skipped ones so comments and references can point at them.
	resolved map[string]*store.Note
	// written lists the records that were created or updated, whose
	// relations are applied once every record exists.
	written []writtenNote
	// spaces caches Space resolution per archive Space UID; memberSpaces is
	// the importer's Space list, loaded on first use.
	spaces       map[string]*int32
	memberSpaces []*store.Space
}

// ImportNoteExport applies an opened archive to the importing user's notes
// following the documented import semantics. The user in ctx must be the
// same as user.
func (s *APIService) ImportNoteExport(ctx context.Context, user *store.User, archive *noteexport.File, policy apipb.ImportNotesRequest_ConflictPolicy) (*apipb.ImportNotesReport, error) {
	if user == nil || archive == nil {
		return nil, errors.New("user and archive are required")
	}
	if policy == apipb.ImportNotesRequest_CONFLICT_POLICY_UNSPECIFIED {
		policy = apipb.ImportNotesRequest_SKIP
	}
	storageSetting, err := s.Store.GetInstanceStorageSetting(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instance storage setting")
	}
	contentLengthLimit, err := s.getContentLengthLimit(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get content length limit")
	}
	existing, err := s.findNotesByUID(ctx, archive, false)
	if err != nil {
		return nil, err
	}
	importer := &noteImporter{
		service:            s,
		ctx:                ctx,
		user:               user,
		archive:            archive,
		policy:             policy,
		report:             &apipb.ImportNotesReport{Warnings: archiveIssues(archive.Warnings)},
		storageSetting:     storageSetting,
		contentLengthLimit: contentLengthLimit,
		existing:           existing,
		resolved:           make(map[string]*store.Note, len(archive.Notes)),
		spaces:             make(map[string]*int32),
	}
	for _, record := range archive.Notes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := importer.importNote(record); err != nil {
			importer.report.Failed++
			importer.report.Failures = append(importer.report.Failures, &apipb.ImportNotesIssue{NoteUid: record.UID, Message: err.Error()})
			continue
		}
		// An imported note counts against the same write budget as one
		// created through CreateNote, so an archive cannot bypass it.
		s.charge(ratelimit.ScopeWriteUser, userKey(user.ID), 1)
	}
	for _, written := range importer.written {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := importer.applyRelations(written); err != nil {
			importer.warn(written.record.UID, "relations were not applied: "+err.Error())
		}
	}
	if importer.report.Created > 0 || importer.report.Updated > 0 {
		s.SSEHub.publishNoteChanged()
	}
	return importer.report, nil
}

func (i *noteImporter) warn(uid, message string) {
	i.report.Warnings = append(i.report.Warnings, &apipb.ImportNotesIssue{NoteUid: uid, Message: message})
}

func (i *noteImporter) importNote(record *noteexport.Note) error {
	content, err := i.archive.Content(record)
	if err != nil {
		return err
	}
	if len(content) > i.contentLengthLimit {
		return errors.Errorf("content too long (max %d bytes)", i.contentLengthLimit)
	}
	targetUID := record.UID
	if existing := i.existing[record.UID]; existing != nil {
		if existing.CreatorID == i.user.ID {
			switch i.policy {
			case apipb.ImportNotesRequest_SKIP:
				i.resolved[record.UID] = existing
				i.report.Skipped++
				return nil
			case apipb.ImportNotesRequest_REPLACE:
				return i.replaceNote(record, existing, content)
			}
		}
		targetUID = shortuuid.New()
	}
	return i.createNote(record, targetUID, content)
}

func (i *noteImporter) createNote(record *noteexport.Note, targetUID string, content []byte) error {
	note, err := i.buildNote(record, targetUID, content)
	if err != nil {
		return err
	}
	var parentID *int32
	if record.Parent != "" {
		parent, err := i.resolveNote(record.Parent)
		if err != nil {
			return err
		}
		if parent == nil {
			i.warn(record.UID, fmt.Sprintf("parent note %s was not found; imported as a top-level note", record.Parent))
		} else {
			parentID = &parent.ID
		}
	}
	added, err := i.importAttachments(record, nil)
	if err != nil {
		return err
	}
	// Creation never sets pinned or the lifecycle state; they follow in a
	// second write, as they do for a note created through the API.
	wantPinned, wantRowStatus := note.Pinned, note.RowStatus
	err = i.bindAttachments(note, added, added, func(prepared *preparedNoteAttachments, required []int32) error {
		if err := i.service.createNoteWithMutation(i.ctx, i.user, note, parentID, prepared, required, nil); err != nil {
			return mapNoteCreateError(err, targetUID, "failed to create note")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if wantPinned || wantRowStatus == store.Archived {
		update := &store.UpdateNote{ID: note.ID, UpdatedTs: &note.UpdatedTs}
		if wantPinned {
			update.Pinned = &wantPinned
		}
		if wantRowStatus == store.Archived {
			update.RowStatus = &wantRowStatus
		}
		if err := i.service.Store.UpdateNote(i.ctx, update); err != nil {
			return mapNoteWriteError(err, "failed to apply note state")
		}
		note.Pinned, note.RowStatus = wantPinned, wantRowStatus
	}
	i.resolved[record.UID] = note
	i.written = append(i.written, writtenNote{record: record, note: note})
	i.report.Created++
	return nil
}

func (i *noteImporter) replaceNote(record *noteexport.Note, existing *store.Note, content []byte) error {
	next, err := i.buildNote(record, existing.UID, content)
	if err != nil {
		return err
	}
	if record.Parent != "" && (existing.ParentUID == nil || *existing.ParentUID != record.Parent) {
		i.warn(record.UID, "comment threading of an existing note cannot be changed; the parent from the archive was ignored")
	}
	update := &store.UpdateNote{
		ID:         existing.ID,
		Content:    &next.Content,
		Payload:    next.Payload,
		Visibility: &next.Visibility,
		Pinned:     &next.Pinned,
		RowStatus:  &next.RowStatus,
		CreatedTs:  &next.CreatedTs,
		UpdatedTs:  &next.UpdatedTs,
		ClearSpace: next.SpaceID == nil,
		SpaceID:    next.SpaceID,
	}
	current, err := i.service.Store.ListAttachments(i.ctx, &store.FindAttachment{NoteID: &existing.ID})
	if err != nil {
		return errors.Wrap(err, "failed to list attachments")
	}
	names := make([]*apipb.Attachment, 0, len(current)+len(record.Attachments))
	for _, attachment := range current {
		names = append(names, &apipb.Attachment{Name: AttachmentNamePrefix + attachment.UID})
	}
	added, err := i.importAttachments(record, current)
	if err != nil {
		return err
	}
	err = i.bindAttachments(existing, append(names, added...), added, func(prepared *preparedNoteAttachments, required []int32) error {
		return i.service.applyNoteMutation(i.ctx, existing, prepared, update, required, nil)
	})
	if err != nil {
		return err
	}
	i.resolved[record.UID] = existing
	i.written = append(i.written, writtenNote{record: record, note: existing})
	i.report.Updated++
	return nil
}

// bindAttachments prepares the note's attachment set, resolves the managed
// references in its content, and runs write. When any step fails, the
// attachments this run created for the note are removed again.
func (i *noteImporter) bindAttachments(note *store.Note, names, added []*apipb.Attachment, write func(*preparedNoteAttachments, []int32) error) error {
	err := func() error {
		prepared, err := i.service.prepareNoteAttachments(i.ctx, i.user, note, names)
		if err != nil {
			return err
		}
		required, err := i.service.resolveNoteAttachmentReferences(note.Content, prepared.normalized)
		if err != nil {
			return err
		}
		return write(prepared, required)
	}()
	if err != nil {
		i.discardAttachments(added)
	}
	return err
}

// buildNote turns a record into the note row to write, resolving the Space
// and applying the visibility fallback.
func (i *noteImporter) buildNote(record *noteexport.Note, uid string, content []byte) (*store.Note, error) {
	createdTs, err := noteexport.ParseTime(record.CreateTime)
	if err != nil {
		return nil, err
	}
	updatedTs, err := noteexport.ParseTime(record.UpdateTime)
	if err != nil {
		return nil, err
	}
	note := &store.Note{
		UID:        uid,
		CreatorID:  i.user.ID,
		RowStatus:  store.RowStatus(record.State),
		CreatedTs:  createdTs,
		UpdatedTs:  updatedTs,
		Content:    string(content),
		Visibility: store.Visibility(record.Visibility),
		Pinned:     record.Pinned,
	}
	if record.Space != nil {
		spaceID, err := i.resolveSpace(record.Space)
		if err != nil {
			return nil, err
		}
		if spaceID == nil {
			i.warn(record.UID, fmt.Sprintf("space %q was not found among your spaces; imported without a space", record.Space.UID))
		}
		note.SpaceID = spaceID
	}
	if note.Visibility == store.SpaceAudience && note.SpaceID == nil {
		i.warn(record.UID, "SPACE visibility needs a space; imported as PRIVATE")
		note.Visibility = store.Private
	}
	if err := notepayload.RebuildNotePayload(i.ctx, note, i.service.MarkdownService); err != nil {
		return nil, errors.Wrap(err, "failed to rebuild note payload")
	}
	if record.Location != nil {
		note.Payload.Location = &storepb.NotePayload_Location{
			Placeholder: record.Location.Placeholder,
			Latitude:    record.Location.Latitude,
			Longitude:   record.Location.Longitude,
		}
	}
	return note, nil
}

// resolveSpace matches a record's Space by UID, then by title, among the
// Spaces the importing user is an active member of.
func (i *noteImporter) resolveSpace(space *noteexport.Space) (*int32, error) {
	if cached, ok := i.spaces[space.UID]; ok {
		return cached, nil
	}
	if i.memberSpaces == nil {
		memberSpaces, err := i.service.Store.ListSpaces(i.ctx, &store.FindSpace{MemberUserID: &i.user.ID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list spaces")
		}
		i.memberSpaces = memberSpaces
	}
	var resolved *int32
	for _, match := range []func(*store.Space) bool{
		func(candidate *store.Space) bool { return candidate.UID == space.UID },
		func(candidate *store.Space) bool { return space.Title != "" && candidate.Title == space.Title },
	} {
		for _, candidate := range i.memberSpaces {
			if candidate.CurrentUserRole.IsActiveMember() && match(candidate) {
				resolved = &candidate.ID
				break
			}
		}
		if resolved != nil {
			break
		}
	}
	i.spaces[space.UID] = resolved
	return resolved, nil
}

// resolveNote finds the instance note an archive UID refers to: one this run
// has already placed, or one on the instance the user may read.
func (i *noteImporter) resolveNote(uid string) (*store.Note, error) {
	if note, ok := i.resolved[uid]; ok {
		return note, nil
	}
	note, err := i.service.Store.GetNote(i.ctx, &store.FindNote{UID: &uid, Access: newNoteAccessScope(i.user, true)})
	if err != nil {
		return nil, errors.Wrap(err, "failed to look up note")
	}
	return note, nil
}

// importAttachments creates the attachments of a record that the note does
// not already hold and returns their names for binding.
func (i *noteImporter) importAttachments(record *noteexport.Note, current []*store.Attachment) ([]*apipb.Attachment, error) {
	bound := make(map[string]struct{}, len(current))
	for _, attachment := range current {
		bound[attachment.UID] = struct{}{}
	}
	toBind := make([]*apipb.Attachment, 0, len(record.Attachments))
	// created holds only attachments this run stored; a pre-existing
	// attachment that is merely re-bound is never discarded on failure.
	created := make([]*apipb.Attachment, 0, len(record.Attachments))
	for index := range record.Attachments {
		entry := &record.Attachments[index]
		if _, ok := bound[entry.UID]; ok {
			continue
		}
		name, stored, err := i.importAttachment(entry)
		if err != nil {
			i.discardAttachments(created)
			return nil, errors.Wrapf(err, "attachment %s", entry.UID)
		}
		attachment := &apipb.Attachment{Name: name}
		toBind = append(toBind, attachment)
		if stored {
			created = append(created, attachment)
			// Stored bytes count against the same upload budget as an upload.
			i.service.charge(ratelimit.ScopeUploadUser, userKey(i.user.ID), 1)
		}
	}
	return toBind, nil
}

// importAttachment stores one attachment entry and returns the resource name
// to bind, and whether this run stored a new attachment for it.
func (i *noteImporter) importAttachment(entry *noteexport.Attachment) (string, bool, error) {
	mimeType, ok := normalizeMimeType(entry.Type)
	if !ok {
		return "", false, errors.Errorf("invalid media type %q", entry.Type)
	}
	if !validateFilename(entry.Filename) {
		return "", false, errors.New("filename contains invalid characters")
	}
	uid, err := i.resolveAttachmentUID(entry)
	if err != nil {
		return "", false, err
	}
	if uid == "" {
		// An identical unlinked attachment already belongs to the user.
		return AttachmentNamePrefix + entry.UID, false, nil
	}
	name, err := i.storeAttachment(entry, uid, mimeType)
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

// storeAttachment writes a new attachment for the entry under uid.
func (i *noteImporter) storeAttachment(entry *noteexport.Attachment, uid, mimeType string) (string, error) {
	if entry.ExternalLink != "" {
		attachment, err := i.service.Store.CreateAttachment(i.ctx, &store.Attachment{
			UID:         uid,
			CreatorID:   i.user.ID,
			Filename:    entry.Filename,
			Type:        mimeType,
			Size:        entry.Size,
			StorageType: storepb.AttachmentStorageType_EXTERNAL,
			Reference:   entry.ExternalLink,
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to create external attachment")
		}
		return AttachmentNamePrefix + attachment.UID, nil
	}
	if err := i.service.checkFileSizeQuota(i.ctx, i.user.ID, entry.Size); err != nil {
		return "", err
	}
	data, err := i.archive.ReadAttachment(entry)
	if err != nil {
		return "", err
	}
	create := &store.Attachment{
		UID:       uid,
		CreatorID: i.user.ID,
		Filename:  entry.Filename,
		Type:      mimeType,
		Size:      int64(len(data)),
	}
	if len(entry.MediaMetadata) > 0 {
		metadata := &apipb.MediaMetadata{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(entry.MediaMetadata, metadata); err != nil {
			i.warn("", fmt.Sprintf("attachment %s: media metadata was not understood and was dropped", entry.UID))
		} else if stored, err := validateClientMediaMetadata(metadata, mimeType); err != nil {
			i.warn("", fmt.Sprintf("attachment %s: media metadata was rejected and dropped: %v", entry.UID, err))
		} else if stored != nil {
			create.Payload = ensureAttachmentPayload(create.Payload)
			create.Payload.MediaMetadata = stored
		}
	}
	attachment, err := i.service.processAndSaveAttachment(i.ctx, create, i.storageSetting, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return attachment.Name, nil
}

// resolveAttachmentUID decides which UID a new attachment gets. It returns
// the empty string when an unlinked attachment with the same UID, owner, and
// bytes already exists and should simply be bound.
func (i *noteImporter) resolveAttachmentUID(entry *noteexport.Attachment) (string, error) {
	existing, err := i.service.Store.GetAttachment(i.ctx, &store.FindAttachment{UID: &entry.UID})
	if err != nil {
		return "", errors.Wrap(err, "failed to look up attachment")
	}
	if existing == nil {
		return entry.UID, nil
	}
	if entry.Path != "" && existing.CreatorID == i.user.ID && existing.NoteID == nil && existing.Size == entry.Size &&
		existing.StorageType != storepb.AttachmentStorageType_EXTERNAL {
		if digest, err := i.storedAttachmentDigest(existing); err == nil && digest == entry.SHA256 {
			return "", nil
		}
	}
	return shortuuid.New(), nil
}

// storedAttachmentDigest streams an attachment from storage and returns its
// lowercase hexadecimal SHA-256.
func (i *noteImporter) storedAttachmentDigest(attachment *store.Attachment) (string, error) {
	content, err := i.service.openAttachmentContent(i.ctx, attachment)
	if err != nil {
		return "", err
	}
	defer content.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, content); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// discardAttachments removes attachments this run created for a note that
// then failed, so no orphan uploads are left behind.
func (i *noteImporter) discardAttachments(attachments []*apipb.Attachment) {
	for _, attachment := range attachments {
		uid, err := ExtractAttachmentUIDFromName(attachment.Name)
		if err != nil {
			continue
		}
		stored, err := i.service.Store.GetAttachment(i.ctx, &store.FindAttachment{UID: &uid})
		if err != nil || stored == nil || stored.CreatorID != i.user.ID || stored.NoteID != nil {
			continue
		}
		if err := i.service.Store.DeleteAttachments(i.ctx, []*store.Attachment{stored}); err != nil {
			slog.Warn("failed to discard attachment after import failure", slog.String("uid", uid), slog.Any("error", err))
		}
	}
}

// applyRelations replaces the note's REFERENCE relations with those from the
// record, skipping targets that exist neither in the archive nor on the
// instance.
func (i *noteImporter) applyRelations(written writtenNote) error {
	record := written.record
	if len(record.Relations) == 0 {
		return nil
	}
	// Reload so the optimistic content check sees the note as written.
	note, err := i.service.Store.GetNote(i.ctx, &store.FindNote{ID: &written.note.ID})
	if err != nil {
		return errors.Wrap(err, "failed to reload note")
	}
	if note == nil {
		return errors.New("note disappeared during import")
	}
	relations := make([]*store.NoteRelation, 0, len(record.Relations))
	seen := map[int32]struct{}{note.ID: {}}
	for _, relation := range record.Relations {
		target, err := i.resolveNote(relation.Note)
		if err != nil {
			return err
		}
		if target == nil {
			i.warn(record.UID, fmt.Sprintf("referenced note %s was not found; the reference was dropped", relation.Note))
			continue
		}
		if _, dup := seen[target.ID]; dup {
			continue
		}
		seen[target.ID] = struct{}{}
		relations = append(relations, &store.NoteRelation{NoteID: note.ID, RelatedNoteID: target.ID, Type: store.NoteRelationReference})
	}
	if len(relations) == 0 {
		return nil
	}
	return i.service.applyNoteMutation(i.ctx, note, nil, nil, nil, &relations)
}

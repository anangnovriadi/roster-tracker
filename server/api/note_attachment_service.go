package api

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/anangnovriadi/roster-tracker/core/access"
	"github.com/anangnovriadi/roster-tracker/markdown"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) SetNoteAttachments(ctx context.Context, request *apipb.SetNoteAttachmentsRequest) (*emptypb.Empty, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := requireNoteWriter(user); err != nil {
		return nil, err
	}
	noteUID, err := ExtractNoteUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note")
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}
	if !access.CanManageNote(user, note) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	prepared, err := s.prepareNoteAttachments(ctx, user, note, request.Attachments)
	if err != nil {
		return nil, err
	}
	requiredAttachmentIDs, err := s.resolveNoteAttachmentReferences(note.Content, prepared.normalized)
	if err != nil {
		return nil, err
	}
	updatedTsSec := time.Now().Unix()
	if err := s.applyNoteMutation(ctx, note, prepared, &store.UpdateNote{ID: note.ID, UpdatedTs: &updatedTsSec}, requiredAttachmentIDs, nil); err != nil {
		return nil, err
	}
	_, _, noteMessage, err := s.buildUpdatedNoteState(ctx, note.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to build updated note state")
	}
	s.dispatchNoteUpdatedSideEffects(ctx, noteMessage)

	return &emptypb.Empty{}, nil
}

type preparedNoteAttachments struct {
	current    []*store.Attachment
	normalized []*store.Attachment
	removed    []*store.Attachment
}

func (s *APIService) prepareNoteAttachments(
	ctx context.Context,
	user *store.User,
	note *store.Note,
	requestAttachments []*apipb.Attachment,
) (*preparedNoteAttachments, error) {
	currentAttachments := []*store.Attachment{}
	if note.ID > 0 {
		var err error
		currentAttachments, err = s.Store.ListAttachments(ctx, &store.FindAttachment{
			NoteID: &note.ID,
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list attachments")
		}
	}

	normalizedAttachments, err := s.normalizeNoteAttachmentRequest(ctx, user, note, currentAttachments, requestAttachments)
	if err != nil {
		return nil, err
	}

	requestedIDs := make(map[int32]bool, len(normalizedAttachments))
	for _, attachment := range normalizedAttachments {
		requestedIDs[attachment.ID] = true
	}

	// The same removal rule is revalidated by the driver in the transaction.
	removedAttachments := make([]*store.Attachment, 0)
	for _, attachment := range currentAttachments {
		if !requestedIDs[attachment.ID] {
			if !store.NoteAttachmentRemovalAllowed(attachment, note.ID, user.ID, access.IsInstanceAdmin(user)) {
				return nil, status.Errorf(codes.PermissionDenied, "cannot remove another user's attachment")
			}
			removedAttachments = append(removedAttachments, attachment)
		}
	}

	return &preparedNoteAttachments{
		current:    currentAttachments,
		normalized: normalizedAttachments,
		removed:    removedAttachments,
	}, nil
}

func buildNoteAttachmentMutationBindings(prepared *preparedNoteAttachments) ([]*store.NoteAttachmentBinding, []int32) {
	if prepared == nil {
		return nil, nil
	}
	currentIDs := make(map[int32]struct{}, len(prepared.current))
	for _, attachment := range prepared.current {
		currentIDs[attachment.ID] = struct{}{}
	}

	normalizedAttachments := slices.Clone(prepared.normalized)
	slices.Reverse(normalizedAttachments)
	bindings := make([]*store.NoteAttachmentBinding, 0, len(normalizedAttachments))
	for index, attachment := range normalizedAttachments {
		_, wasBoundToNote := currentIDs[attachment.ID]
		bindings = append(bindings, &store.NoteAttachmentBinding{
			ID:             attachment.ID,
			UID:            attachment.UID,
			UpdatedTs:      time.Now().Unix() + int64(index),
			WasBoundToNote: wasBoundToNote,
		})
	}
	slices.SortFunc(bindings, func(a, b *store.NoteAttachmentBinding) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	removedAttachmentIDs := make([]int32, 0, len(prepared.removed))
	for _, attachment := range prepared.removed {
		removedAttachmentIDs = append(removedAttachmentIDs, attachment.ID)
	}
	return bindings, removedAttachmentIDs
}

// createNoteWithMutation creates the note row, binds attachments, and writes
// reference relations in one database transaction. A failure in any dependent
// write therefore cannot leave an orphan note behind.
func (s *APIService) createNoteWithMutation(
	ctx context.Context,
	user *store.User,
	create *store.Note,
	commentContextNoteID *int32,
	prepared *preparedNoteAttachments,
	requiredAttachmentIDs []int32,
	referenceRelations []*store.NoteRelation,
) error {
	if user == nil || create == nil || create.CreatorID != user.ID {
		return store.ErrNotePermissionDenied
	}
	bindings, _ := buildNoteAttachmentMutationBindings(prepared)
	relations := make([]*store.NoteRelation, 0, len(referenceRelations))
	for _, relation := range referenceRelations {
		if relation == nil {
			continue
		}
		relations = append(relations, &store.NoteRelation{
			RelatedNoteID: relation.RelatedNoteID,
			Type:          relation.Type,
		})
	}
	return s.Store.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteCreate:                create,
		CommentContextNoteID:      commentContextNoteID,
		NoteCreatorID:             create.CreatorID,
		ExpectedNoteContent:       create.Content,
		Bindings:                  bindings,
		RequiredAttachmentIDs:     requiredAttachmentIDs,
		ReplaceReferenceRelations: len(relations) > 0,
		ReferenceRelations:        relations,
	})
}

func (s *APIService) applyNoteMutation(
	ctx context.Context,
	note *store.Note,
	prepared *preparedNoteAttachments,
	noteUpdate *store.UpdateNote,
	requiredAttachmentIDs []int32,
	referenceRelations *[]*store.NoteRelation,
) error {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return status.Error(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return status.Error(codes.Unauthenticated, "user not authenticated")
	}
	if !access.CanManageNote(user, note) {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	policy := noteWritePolicy(user.ID, false)
	if noteUpdate != nil && noteUpdate.Policy != nil {
		policy = noteUpdate.Policy
	} else if noteUpdate != nil {
		noteUpdate.Policy = policy
	}
	if prepared == nil {
		prepared = &preparedNoteAttachments{}
	}
	if len(prepared.removed) > 0 {
		if _, err := s.validateAttachmentMotionGroupDeletion(ctx, user, prepared.removed); err != nil {
			return err
		}
	}
	bindings, removedAttachmentIDs := buildNoteAttachmentMutationBindings(prepared)
	if referenceRelations != nil {
		relations := make([]*store.NoteRelation, 0, len(*referenceRelations))
		for _, relation := range *referenceRelations {
			relations = append(relations, &store.NoteRelation{
				NoteID:        note.ID,
				RelatedNoteID: relation.RelatedNoteID,
				Type:          relation.Type,
			})
		}
		referenceRelations = &relations
	}
	mutation := &store.NoteMutation{
		NoteID:                    note.ID,
		NoteCreatorID:             note.CreatorID,
		ExpectedNoteContent:       note.Content,
		NoteUpdate:                noteUpdate,
		Bindings:                  bindings,
		RemovedAttachmentIDs:      removedAttachmentIDs,
		RequiredAttachmentIDs:     requiredAttachmentIDs,
		ReplaceReferenceRelations: referenceRelations != nil,
		Policy:                    policy,
	}
	if referenceRelations != nil {
		mutation.ReferenceRelations = *referenceRelations
	}
	if err := s.Store.ApplyNoteMutation(ctx, mutation); err != nil {
		return mapNoteWriteError(err, "failed to apply note mutation")
	}
	if err := s.cleanupDeletedAttachmentStorage(ctx, prepared.removed); err != nil {
		return status.Errorf(codes.Internal, "note was updated but attachment storage cleanup failed: %v", err)
	}
	return nil
}

func (s *APIService) normalizeNoteAttachmentRequest(
	ctx context.Context,
	user *store.User,
	note *store.Note,
	currentAttachments []*store.Attachment,
	requestAttachments []*apipb.Attachment,
) ([]*store.Attachment, error) {
	currentByID := make(map[int32]struct{}, len(currentAttachments))
	for _, attachment := range currentAttachments {
		currentByID[attachment.ID] = struct{}{}
	}

	requestedAttachments := make([]*store.Attachment, 0, len(requestAttachments))
	for _, requestAttachment := range requestAttachments {
		if requestAttachment == nil {
			return nil, status.Errorf(codes.InvalidArgument, "attachment is required")
		}
		attachmentUID, err := ExtractAttachmentUIDFromName(requestAttachment.Name)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid attachment name: %v", err)
		}
		attachment, err := s.Store.GetAttachment(ctx, &store.FindAttachment{UID: &attachmentUID})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get attachment: %v", err)
		}
		if attachment == nil {
			return nil, status.Errorf(codes.NotFound, "attachment not found: %s", attachmentUID)
		}
		_, alreadyInTarget := currentByID[attachment.ID]
		if !alreadyInTarget && !store.NoteAttachmentBindingOwnerAllowed(attachment.CreatorID, note.CreatorID, user.ID, access.IsInstanceAdmin(user)) {
			// Hide another user's unlinked or foreign-note attachment from the
			// caller. A new binding must be owned by the note author or by the
			// acting administrator; ownership is never transferred between
			// accounts.
			return nil, status.Errorf(codes.NotFound, "attachment not found: %s", attachmentUID)
		}
		if attachment.NoteID != nil && !alreadyInTarget {
			return nil, status.Errorf(codes.FailedPrecondition, "attachment %s is already bound to another note", attachmentUID)
		}
		requestedAttachments = append(requestedAttachments, attachment)
	}

	currentGroups := make(map[string][]*store.Attachment)
	for _, attachment := range currentAttachments {
		motion := getAttachmentMotionMedia(attachment)
		if motion == nil || motion.GroupId == "" {
			continue
		}
		currentGroups[motion.GroupId] = append(currentGroups[motion.GroupId], attachment)
	}

	requestGroups := make(map[string][]*store.Attachment)
	requestNamesByGroup := make(map[string]map[string]bool)
	for _, attachment := range requestedAttachments {
		motion := getAttachmentMotionMedia(attachment)
		if motion == nil || motion.GroupId == "" {
			continue
		}
		requestGroups[motion.GroupId] = append(requestGroups[motion.GroupId], attachment)
		if requestNamesByGroup[motion.GroupId] == nil {
			requestNamesByGroup[motion.GroupId] = make(map[string]bool)
		}
		requestNamesByGroup[motion.GroupId][attachment.UID] = true
	}

	normalized := make([]*store.Attachment, 0, len(requestedAttachments))
	appendedGroups := make(map[string]bool)
	appendedAttachments := make(map[string]bool)
	for _, attachment := range requestedAttachments {
		motion := getAttachmentMotionMedia(attachment)
		if motion == nil || motion.GroupId == "" {
			if !appendedAttachments[attachment.UID] {
				normalized = append(normalized, attachment)
				appendedAttachments[attachment.UID] = true
			}
			continue
		}

		groupID := motion.GroupId
		if appendedGroups[groupID] {
			continue
		}

		currentGroup := currentGroups[groupID]
		if isMultiMemberMotionGroup(currentGroup) && !allGroupMembersRequested(currentGroup, requestNamesByGroup[groupID]) {
			appendedGroups[groupID] = true
			continue
		}

		for _, groupAttachment := range requestGroups[groupID] {
			if appendedAttachments[groupAttachment.UID] {
				continue
			}
			normalized = append(normalized, groupAttachment)
			appendedAttachments[groupAttachment.UID] = true
		}
		appendedGroups[groupID] = true
	}

	return normalized, nil
}

func (s *APIService) resolveNoteAttachmentReferences(content string, attachments []*store.Attachment) ([]int32, error) {
	references, err := s.extractManagedAttachmentReferences(content)
	if err != nil {
		return nil, err
	}

	attachmentsByUID := make(map[string]*store.Attachment, len(attachments))
	for _, attachment := range attachments {
		attachmentsByUID[attachment.UID] = attachment
	}
	requiredAttachmentIDs := make([]int32, 0, len(references))
	for _, reference := range references {
		attachment := attachmentsByUID[reference.UID]
		if attachment == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "managed image attachment %s must be bound to the note", reference.UID)
		}
		if !strings.HasPrefix(strings.ToLower(attachment.Type), "image/") {
			return nil, status.Errorf(codes.InvalidArgument, "managed image attachment %s must have an image MIME type", reference.UID)
		}
		if attachment.StorageType == storepb.AttachmentStorageType_EXTERNAL {
			return nil, status.Errorf(codes.InvalidArgument, "external attachment %s cannot be used as a managed note image", reference.UID)
		}
		requiredAttachmentIDs = append(requiredAttachmentIDs, attachment.ID)
	}
	return requiredAttachmentIDs, nil
}

func (s *APIService) extractManagedAttachmentReferences(content string) ([]markdown.ManagedAttachmentReference, error) {
	data, err := s.MarkdownService.ExtractAll([]byte(content))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid markdown content: %v", err)
	}
	if len(data.InvalidManagedAttachmentReferences) > 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid managed attachment image URL: %s", data.InvalidManagedAttachmentReferences[0])
	}
	references := slices.Clone(data.ManagedAttachmentReferences)
	instanceURLString := ""
	if s.Profile != nil {
		instanceURLString = strings.TrimSpace(s.Profile.InstanceURL)
	}
	var instanceURL *url.URL
	if instanceURLString != "" {
		instanceURL, err = url.Parse(instanceURLString)
		if err != nil || !instanceURL.IsAbs() || instanceURL.Host == "" {
			return nil, status.Errorf(codes.Internal, "invalid configured instance URL")
		}
	}
	for _, destination := range data.ImageDestinations {
		parsed, err := url.Parse(destination)
		if err != nil || (!parsed.IsAbs() && parsed.Host == "") || !strings.HasPrefix(parsed.Path, "/file/attachments/") {
			continue
		}
		if parsed.Scheme == "" && parsed.Host != "" {
			if instanceURL == nil || sameURLAuthority(parsed, instanceURL) {
				return nil, status.Errorf(codes.InvalidArgument, "protocol-relative managed attachment image URLs are not allowed: %s", destination)
			}
			continue
		}
		if instanceURL == nil {
			return nil, status.Errorf(codes.InvalidArgument, "absolute managed attachment image URLs require a configured instance URL")
		}
		if !sameURLOrigin(parsed, instanceURL) {
			continue
		}
		relative := parsed.EscapedPath()
		if parsed.RawQuery != "" {
			relative += "?" + parsed.RawQuery
		}
		if parsed.Fragment != "" {
			relative += "#" + parsed.Fragment
		}
		uid, managed, valid := markdown.ParseManagedAttachmentImageURL(relative)
		if !managed || !valid {
			return nil, status.Errorf(codes.InvalidArgument, "invalid managed attachment image URL: %s", destination)
		}
		references = append(references, markdown.ManagedAttachmentReference{UID: uid})
	}

	seenReferences := make(map[string]struct{}, len(references))
	uniqueReferences := make([]markdown.ManagedAttachmentReference, 0, len(references))
	for _, reference := range references {
		if _, ok := seenReferences[reference.UID]; ok {
			continue
		}
		seenReferences[reference.UID] = struct{}{}
		uniqueReferences = append(uniqueReferences, reference)
	}
	return uniqueReferences, nil
}

func sameURLAuthority(candidate, instance *url.URL) bool {
	withScheme := candidate.Clone()
	withScheme.Scheme = instance.Scheme
	return sameURLOrigin(withScheme, instance)
}

func sameURLOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectiveURLPort(a) == effectiveURLPort(b)
}

func effectiveURLPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func allGroupMembersRequested(group []*store.Attachment, requestedNames map[string]bool) bool {
	if len(group) == 0 {
		return false
	}

	for _, attachment := range group {
		if !requestedNames[attachment.UID] {
			return false
		}
	}
	return true
}

func (s *APIService) ListNoteAttachments(ctx context.Context, request *apipb.ListNoteAttachmentsRequest) (*apipb.ListNoteAttachmentsResponse, error) {
	noteUID, err := ExtractNoteUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid note name: %v", err)
	}
	note, err := s.Store.GetNote(ctx, &store.FindNote{UID: &noteUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get note: %v", err)
	}
	if note == nil {
		return nil, status.Errorf(codes.NotFound, "note not found")
	}

	if err := s.checkNoteReadAccess(ctx, note); err != nil {
		return nil, err
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		NoteID: &note.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments: %v", err)
	}

	response := &apipb.ListNoteAttachmentsResponse{
		Attachments: []*apipb.Attachment{},
	}
	for _, attachment := range attachments {
		response.Attachments = append(response.Attachments, convertAttachmentFromStore(attachment))
	}
	return response, nil
}

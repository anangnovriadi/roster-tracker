package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestAttachmentAccessScopeFiltersBeforePagination(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-space-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "attachment-space-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "attachment-space", Title: "Attachments"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)
	root, err := ts.CreateNote(ctx, &store.Note{UID: "attachment-space-root", CreatorID: owner.ID, Content: "root", Visibility: store.SpaceAudience, SpaceID: &space.ID})
	require.NoError(t, err)
	comment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "attachment-space-comment", CreatorID: owner.ID, Content: "comment", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	}, root.ID, owner.ID)
	require.NoError(t, err)

	unlinked, err := ts.CreateAttachment(ctx, &store.Attachment{UID: "attachment-unlinked", CreatorID: member.ID, Filename: "unlinked.txt", Type: "text/plain", Size: 1, Blob: []byte("u")})
	require.NoError(t, err)
	linked, err := ts.CreateAttachment(ctx, &store.Attachment{UID: "attachment-linked", CreatorID: member.ID, Filename: "linked.txt", Type: "text/plain", Size: 1, Blob: []byte("l"), NoteID: &comment.ID})
	require.NoError(t, err)
	older, newer := time.Now().Unix()-20, time.Now().Unix()-10
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: unlinked.ID, UpdatedTs: &older}))
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: linked.ID, UpdatedTs: &newer}))
	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: member.ID}, owner.ID))

	limit, offset := 1, 0
	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &member.ID,
		Access: &store.NoteAccessScope{
			UserID:         &member.ID,
			AllowPublic:    true,
			AllowProtected: true,
		},
		Limit:  &limit,
		Offset: &offset,
	})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, unlinked.ID, attachments[0].ID, "the inaccessible newer row must be removed before LIMIT is applied")
}

func TestAttachmentSpaceFilterFiltersByNotePlacement(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-scope-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	spaceA, err := ts.CreateSpace(ctx, &store.Space{UID: "attachment-scope-a", Title: "A"}, owner.ID)
	require.NoError(t, err)
	spaceB, err := ts.CreateSpace(ctx, &store.Space{UID: "attachment-scope-b", Title: "B"}, owner.ID)
	require.NoError(t, err)

	unassignedNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-scope-unassigned", CreatorID: owner.ID, Content: "unassigned", Visibility: store.Private,
	})
	require.NoError(t, err)
	spaceANote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-scope-a-note", CreatorID: owner.ID, Content: "a", Visibility: store.Private, SpaceID: &spaceA.ID,
	})
	require.NoError(t, err)
	spaceBNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-scope-b-note", CreatorID: owner.ID, Content: "b", Visibility: store.Public, SpaceID: &spaceB.ID,
	})
	require.NoError(t, err)

	createAttachment := func(uid string, noteID *int32) *store.Attachment {
		t.Helper()
		attachment, createErr := ts.CreateAttachment(ctx, &store.Attachment{
			UID: uid, CreatorID: owner.ID, Filename: uid + ".txt", Type: "text/plain", NoteID: noteID,
		})
		require.NoError(t, createErr)
		return attachment
	}
	unlinked := createAttachment("attachment-scope-unlinked", nil)
	unassigned := createAttachment("attachment-scope-unassigned-file", &unassignedNote.ID)
	inSpaceANewer := createAttachment("attachment-scope-a-newer", &spaceANote.ID)
	inSpaceAOlder := createAttachment("attachment-scope-a-older", &spaceANote.ID)
	inSpaceB := createAttachment("attachment-scope-b-file", &spaceBNote.ID)

	baseTime := time.Now().Unix() - 100
	for attachment, updatedTs := range map[*store.Attachment]int64{
		inSpaceB:      baseTime + 50,
		inSpaceANewer: baseTime + 40,
		unlinked:      baseTime + 30,
		inSpaceAOlder: baseTime + 20,
		unassigned:    baseTime + 10,
	} {
		require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: attachment.ID, UpdatedTs: &updatedTs}))
	}

	access := &store.NoteAccessScope{UserID: &owner.ID, AllowPublic: true, AllowProtected: true}
	limit, firstOffset := 1, 0
	spaceAttachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &owner.ID, Access: access, Filters: []string{`space == "spaces/attachment-scope-a"`}, Limit: &limit, Offset: &firstOffset,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{inSpaceANewer.ID}, attachmentIDs(spaceAttachments), "filter must be applied before the first page")
	secondOffset := 1
	spaceAttachments, err = ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &owner.ID, Access: access, Filters: []string{`space == "spaces/attachment-scope-a"`}, Limit: &limit, Offset: &secondOffset,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{inSpaceAOlder.ID}, attachmentIDs(spaceAttachments), "filter must be applied before the second page")

	unassignedAttachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &owner.ID, Access: access, Filters: []string{`space == null`}, SkipDefaultLimit: true,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []int32{unlinked.ID, unassigned.ID}, attachmentIDs(unassignedAttachments))
}

func TestAttachmentCreatorFilterUsesNoteAuthor(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	uploader, err := ts.CreateUser(ctx, &store.User{Username: "attachment-uploader", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	author, err := ts.CreateUser(ctx, &store.User{Username: "attachment-note-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-creator-note", CreatorID: author.ID, Content: "public note", Visibility: store.Public,
	})
	require.NoError(t, err)
	linked, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-creator-linked", CreatorID: uploader.ID, Filename: "linked.txt", Type: "text/plain", NoteID: &note.ID,
	})
	require.NoError(t, err)
	unlinked, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-creator-unlinked", CreatorID: uploader.ID, Filename: "unlinked.txt", Type: "text/plain",
	})
	require.NoError(t, err)

	access := &store.NoteAccessScope{UserID: &uploader.ID, AllowPublic: true, AllowProtected: true}
	list := func(filter string) []*store.Attachment {
		attachments, listErr := ts.ListAttachments(ctx, &store.FindAttachment{
			Access: access, Filters: []string{filter}, SkipDefaultLimit: true,
		})
		require.NoError(t, listErr)
		return attachments
	}
	require.Equal(t, []int32{linked.ID}, attachmentIDs(list(`creator == "users/attachment-note-author"`)))
	require.Equal(t, []int32{unlinked.ID}, attachmentIDs(list(`creator == "users/attachment-uploader"`)))
}

func TestAttachmentAccessScopeUsesDirectNoteAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-local-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	contextNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-local-context", CreatorID: owner.ID, Content: "private context", Visibility: store.Private,
	})
	require.NoError(t, err)
	publicComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "attachment-local-public", CreatorID: owner.ID, Content: "public reply", Visibility: store.Public,
	}, contextNote.ID, owner.ID)
	require.NoError(t, err)
	privateComment, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "attachment-local-private", CreatorID: owner.ID, Content: "private reply", Visibility: store.Private,
	}, contextNote.ID, owner.ID)
	require.NoError(t, err)
	publicAttachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-local-public-file", CreatorID: owner.ID, Filename: "public.txt", Type: "text/plain", NoteID: &publicComment.ID,
	})
	require.NoError(t, err)
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-local-private-file", CreatorID: owner.ID, Filename: "private.txt", Type: "text/plain", NoteID: &privateComment.ID,
	})
	require.NoError(t, err)

	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		Access:           &store.NoteAccessScope{AllowPublic: true},
		SkipDefaultLimit: true,
	})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, publicAttachment.ID, attachments[0].ID, "the context note must not grant or restrict attachment access")
}

func TestAttachmentAccessScopeSpaceAudienceHasNoNoteAuthorBypass(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	author, err := ts.CreateUser(ctx, &store.User{Username: "attachment-removed-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	admin, err := ts.CreateUser(ctx, &store.User{Username: "attachment-remaining-admin", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "attachment-author-space", Title: "Author attachment access"}, author.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin}, author.ID)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-author-note", CreatorID: author.ID, Content: "members", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-author-file", CreatorID: author.ID, Filename: "members.txt", Type: "text/plain", NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: author.ID}, admin.ID))

	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID:        &author.ID,
		Access:           &store.NoteAccessScope{UserID: &author.ID, AllowPublic: true, AllowProtected: true},
		SkipDefaultLimit: true,
	})
	require.NoError(t, err)
	require.Empty(t, attachments)
}

func TestAttachmentAccessScopeAllowsArchivedNoteCreatorAndFiltersMissingCreator(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	creator, err := ts.CreateUser(ctx, &store.User{Username: "attachment-archived-note-creator", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	attachmentOwner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-creator-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	validNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-archived-creator-note", CreatorID: creator.ID, Content: "valid", Visibility: store.Public,
	})
	require.NoError(t, err)
	validAttachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-archived-creator-file", CreatorID: attachmentOwner.ID, Filename: "valid.txt", Type: "text/plain", NoteID: &validNote.ID,
	})
	require.NoError(t, err)
	archived := store.Archived
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: creator.ID, RowStatus: &archived})
	require.NoError(t, err)

	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		Access:           &store.NoteAccessScope{AllowPublic: true},
		SkipDefaultLimit: true,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{validAttachment.ID}, attachmentIDs(attachments),
		"attachments keep following an active note's audience when its creator is archived")

	corruptNote, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-missing-creator-note", CreatorID: attachmentOwner.ID, Content: "corrupt", Visibility: store.Public,
	})
	require.NoError(t, err)
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-missing-creator-file", CreatorID: attachmentOwner.ID, Filename: "corrupt.txt", Type: "text/plain", NoteID: &corruptNote.ID,
	})
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx,
		fmt.Sprintf("UPDATE note SET creator_id = 2147483000 WHERE id = %d", corruptNote.ID))
	require.NoError(t, err)

	attachments, err = ts.ListAttachments(ctx, &store.FindAttachment{
		Access:           &store.NoteAccessScope{AllowPublic: true},
		SkipDefaultLimit: true,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{validAttachment.ID}, attachmentIDs(attachments),
		"an attachment must not reveal a note whose creator cannot be resolved")
}

func TestAttachmentWritePolicyRevalidatesNoteSpaceMembership(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-write-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "attachment-write-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	space, err := ts.CreateSpace(ctx, &store.Space{UID: "attachment-write-space", Title: "Attachment Writes"}, owner.ID)
	require.NoError(t, err)
	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)

	root, err := ts.CreateNote(ctx, &store.Note{
		UID: "attachment-write-root", CreatorID: member.ID, Content: "root", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	})
	require.NoError(t, err)
	target, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "attachment-write-comment", CreatorID: member.ID, Content: "comment", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	}, root.ID, member.ID)
	require.NoError(t, err)
	secondTarget, err := ts.CreateNoteComment(ctx, &store.Note{
		UID: "attachment-write-second-comment", CreatorID: member.ID, Content: "second", Visibility: store.SpaceAudience, SpaceID: &space.ID,
	}, root.ID, member.ID)
	require.NoError(t, err)
	policy := noteWritePolicy(member.ID, false)
	unlinked, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-policy-unlinked", CreatorID: member.ID, Filename: "unlinked.txt", Type: "text/plain",
	})
	require.NoError(t, err)
	unlinkedRename := "unlinked-renamed.txt"
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: unlinked.ID, Filename: &unlinkedRename, Policy: policy,
	}))

	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "attachment-policy-linked",
		CreatorID: member.ID,
		Filename:  "linked.txt",
		Type:      "text/plain",
		Size:      1,
		Blob:      []byte("a"),
		NoteID:    &target.ID,
		Policy:    policy,
	})
	require.NoError(t, err)
	renamed := "renamed.txt"
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: attachment.ID, Filename: &renamed, Policy: policy,
	}))
	foreignAttachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "attachment-policy-foreign-owner",
		CreatorID: owner.ID,
		Filename:  "foreign.txt",
		Type:      "text/plain",
		NoteID:    &target.ID,
	})
	require.NoError(t, err)
	foreignRename := "must-not-rename-foreign.txt"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: foreignAttachment.ID, Filename: &foreignRename, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrNotePermissionDenied)
	storedForeign, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &foreignAttachment.ID})
	require.NoError(t, err)
	require.Equal(t, "foreign.txt", storedForeign.Filename)

	// A trusted internal caller can still rebind legacy data. A later transport
	// update discovers and authorizes the current binding in its transaction.
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: attachment.ID, NoteID: &secondTarget.ID}))
	currentBindingRename := "current-binding.txt"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: attachment.ID, Filename: &currentBindingRename, Policy: policy,
	})
	require.NoError(t, err)
	stored, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Equal(t, currentBindingRename, stored.Filename)
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: attachment.ID, NoteID: &target.ID}))

	archived := store.Archived
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: member.ID, RowStatus: &archived})
	require.NoError(t, err)
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "attachment-policy-archived-actor",
		CreatorID: member.ID,
		Filename:  "archived.txt",
		Type:      "text/plain",
		NoteID:    &target.ID,
		Policy:    policy,
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)
	archivedRename := "archived-must-not-rename.txt"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: unlinked.ID, Filename: &archivedRename, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrNotePermissionDenied)
	normal := store.Normal
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: member.ID, RowStatus: &normal})
	require.NoError(t, err)

	require.NoError(t, ts.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: member.ID}, owner.ID))
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "attachment-policy-revoked-create",
		CreatorID: member.ID,
		Filename:  "revoked.txt",
		Type:      "text/plain",
		NoteID:    &target.ID,
		Policy:    policy,
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)
	revokedRename := "revoked-rename.txt"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: attachment.ID, Filename: &revokedRename, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrNoteSpaceMembershipRequired)
	stored, err = ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Equal(t, currentBindingRename, stored.Filename)

	_, err = createSpaceMemberForTest(ctx, ts, &store.SpaceMember{SpaceID: space.ID, UserID: member.ID, Role: store.SpaceMemberRoleUser}, owner.ID)
	require.NoError(t, err)
	restoredRename := "restored-membership.txt"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: attachment.ID, Filename: &restoredRename, Policy: policy,
	})
	require.NoError(t, err)
	stored, err = ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Equal(t, restoredRename, stored.Filename)
}

func attachmentIDs(attachments []*store.Attachment) []int32 {
	ids := make([]int32, 0, len(attachments))
	for _, attachment := range attachments {
		ids = append(ids, attachment.ID)
	}
	return ids
}

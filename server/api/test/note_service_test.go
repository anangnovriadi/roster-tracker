package test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestAnonymousNoteAccessFollowsInstanceAccessSetting(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "access-mode-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "public note",
		Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)

	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
	_, err = ts.Service.ListNotes(ctx, &apipb.ListNotesRequest{})
	require.NoError(t, err)

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))
	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: note.Name})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = ts.Service.ListNotes(ctx, &apipb.ListNotesRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC))
	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
}

func TestInstanceAdminIsSuperuserForNamedNoteOperations(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "superuser-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	admin, err := ts.CreateHostUser(ctx, "superuser-admin")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)
	other, err := ts.CreateRegularUser(ctx, "superuser-other")
	require.NoError(t, err)
	otherCtx := ts.CreateUserContext(ctx, other.ID)

	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "superuser-space", Title: "Superuser"}, owner.ID)
	require.NoError(t, err)
	private, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "private note",
		Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	spaceNote, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "members only",
		Visibility: apipb.Visibility_SPACE,
		Space:      new("spaces/" + space.UID),
	}})
	require.NoError(t, err)

	// Point reads bypass audience and membership for the administrator only.
	_, err = ts.Service.GetNote(otherCtx, &apipb.GetNoteRequest{Name: private.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	got, err := ts.Service.GetNote(adminCtx, &apipb.GetNoteRequest{Name: private.Name})
	require.NoError(t, err)
	require.Equal(t, "private note", got.Content)
	got, err = ts.Service.GetNote(adminCtx, &apipb.GetNoteRequest{Name: spaceNote.Name})
	require.NoError(t, err, "a SPACE note is readable by name without membership")
	require.Equal(t, "spaces/"+space.UID, got.GetSpace())

	// Collections keep the audience predicate.
	feed, err := ts.Service.ListNotes(adminCtx, &apipb.ListNotesRequest{PageSize: 50})
	require.NoError(t, err)
	for _, candidate := range feed.Notes {
		require.NotEqual(t, private.Name, candidate.Name, "another user's PRIVATE note must not surface in the administrator's feed")
		require.NotEqual(t, spaceNote.Name, candidate.Name, "a SPACE note must not surface without membership")
	}
	_, err = ts.Service.ListNotes(adminCtx, &apipb.ListNotesRequest{Filter: `space == "spaces/` + space.UID + `"`})
	require.Equal(t, codes.NotFound, status.Code(err), "the Space feed stays membership-only")

	// Named mutations: content, audience, placement, comments, and deletion.
	_, err = ts.Service.UpdateNote(otherCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: private.Name, Content: "hijacked"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	updated, err := ts.Service.UpdateNote(adminCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: private.Name, Content: "edited by admin"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	require.Equal(t, "edited by admin", updated.Content)
	require.Equal(t, "users/"+owner.Username, updated.Creator, "authorship is unchanged")
	_, err = ts.Service.UpdateNote(adminCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: spaceNote.Name, Content: "edited without membership"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	_, err = ts.Service.UpdateNote(adminCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: private.Name, Visibility: apipb.Visibility_SPACE, Space: new("spaces/" + space.UID)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"visibility", "space"}},
	})
	require.NoError(t, err, "an administrator places notes in a Space they are not a member of")
	_, err = ts.Service.UpdateNote(adminCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: private.Name, Space: new("spaces/does-not-exist")},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.Equal(t, codes.NotFound, status.Code(err), "structural validity still applies")
	_, err = ts.Service.CreateNoteComment(adminCtx, &apipb.CreateNoteCommentRequest{
		Parent:  spaceNote.Name,
		Comment: &apipb.Note{Content: "admin comment"},
	})
	require.NoError(t, err, "participation does not require membership")

	_, err = ts.Service.DeleteNote(otherCtx, &apipb.DeleteNoteRequest{Name: private.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = ts.Service.DeleteNote(adminCtx, &apipb.DeleteNoteRequest{Name: private.Name})
	require.NoError(t, err)
	_, err = ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: private.Name})
	require.Equal(t, codes.NotFound, status.Code(err))

	// An archived administrator holds no privilege. Another administrator
	// must remain for the instance to allow the archive.
	_, err = ts.CreateHostUser(ctx, "superuser-admin-successor")
	require.NoError(t, err)
	archived := store.Archived
	_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{ID: admin.ID, RowStatus: &archived})
	require.NoError(t, err)
	_, err = ts.Service.GetNote(adminCtx, &apipb.GetNoteRequest{Name: spaceNote.Name})
	require.Error(t, err)
}

func TestNoteReadAllowsArchivedCreatorAccordingToAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	creator, err := ts.CreateRegularUser(ctx, "archived-note-creator")
	require.NoError(t, err)
	viewer, err := ts.CreateRegularUser(ctx, "archived-note-viewer")
	require.NoError(t, err)
	creatorCtx := ts.CreateUserContext(ctx, creator.ID)
	viewerCtx := ts.CreateUserContext(ctx, viewer.ID)
	publicNote, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "public note by archived creator", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	protectedNote, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "protected note by archived creator", Visibility: apipb.Visibility_PROTECTED,
	}})
	require.NoError(t, err)
	archived := store.Archived
	_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{ID: creator.ID, RowStatus: &archived})
	require.NoError(t, err)

	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: publicNote.Name})
	require.NoError(t, err, "creator archive must not narrow a PUBLIC note")
	_, err = ts.Service.GetNote(viewerCtx, &apipb.GetNoteRequest{Name: protectedNote.Name})
	require.NoError(t, err, "creator archive must not narrow a PROTECTED note")
}

func TestDanglingPlacementDoesNotOverrideNonSpaceAudience(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	creator, err := ts.CreateRegularUser(ctx, "dangling-placement-creator")
	require.NoError(t, err)
	creatorCtx := ts.CreateUserContext(ctx, creator.ID)
	note, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "public note with corrupt placement", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	noteID := parseNoteIDFromNameForTest(t, ts, note.Name)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx,
		fmt.Sprintf("UPDATE note SET space_id = 2147483000 WHERE id = %d", noteID))
	require.NoError(t, err)

	got, err := ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
	require.Nil(t, got.Space, "an invalid placement must be omitted")

	listed, err := ts.Service.ListNotes(ctx, &apipb.ListNotesRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Notes, 1)
	require.Equal(t, note.Name, listed.Notes[0].Name)
	require.Nil(t, listed.Notes[0].Space)

	_, err = ts.Service.CreateNoteComment(creatorCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name, Comment: &apipb.Note{Content: "must not participate", Visibility: apipb.Visibility_PUBLIC},
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "placement-dependent participation must fail closed")
}

func TestNoteReadFailsClosedForMissingCreatorBeforePaginationAndChildren(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	creator, err := ts.CreateRegularUser(ctx, "missing-note-creator")
	require.NoError(t, err)
	creatorCtx := ts.CreateUserContext(ctx, creator.ID)
	first, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "first valid", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	second, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "second valid", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	corrupt, err := ts.Service.CreateNote(creatorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "dangling creator", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	_, err = ts.Service.UpsertNoteReaction(creatorCtx, &apipb.UpsertNoteReactionRequest{
		Parent: corrupt.Name, Reaction: &apipb.Reaction{ReactionType: "eyes"},
	})
	require.NoError(t, err)
	_, err = ts.Service.CreateNoteComment(creatorCtx, &apipb.CreateNoteCommentRequest{
		Parent: corrupt.Name, Comment: &apipb.Note{Content: "existing comment", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	corruptID := parseNoteIDFromNameForTest(t, ts, corrupt.Name)
	_, err = ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID: "missing-note-creator-attachment", CreatorID: creator.ID, Filename: "secret.txt", Type: "text/plain", NoteID: &corruptID,
	})
	require.NoError(t, err)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx,
		fmt.Sprintf("UPDATE note SET creator_id = 2147483000 WHERE id = %d", corruptID))
	require.NoError(t, err)

	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: corrupt.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = ts.Service.ListNoteReactions(ctx, &apipb.ListNoteReactionsRequest{Parent: corrupt.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = ts.Service.ListNoteAttachments(ctx, &apipb.ListNoteAttachmentsRequest{Parent: corrupt.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = ts.Service.ListNoteComments(ctx, &apipb.ListNoteCommentsRequest{Parent: corrupt.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = ts.Service.CreateNoteComment(creatorCtx, &apipb.CreateNoteCommentRequest{
		Parent: corrupt.Name, Comment: &apipb.Note{Content: "must not be created", Visibility: apipb.Visibility_PUBLIC},
	})
	require.Equal(t, codes.NotFound, status.Code(err))

	listed, err := ts.Service.ListNotes(ctx, &apipb.ListNotesRequest{PageSize: 2})
	require.NoError(t, err)
	require.Len(t, listed.Notes, 2)
	require.ElementsMatch(t, []string{first.Name, second.Name}, []string{listed.Notes[0].Name, listed.Notes[1].Name},
		"a newer dangling-creator row must be filtered before the page limit")
}

func TestCreateNoteAcceptsUUID(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	const noteID = "21ec98aa-9a8f-458c-a2a3-c7dc69b6f591"
	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Created with a UUID",
			Visibility: apipb.Visibility_PRIVATE,
		},
		NoteId: noteID,
	})
	require.NoError(t, err)
	require.Equal(t, "notes/"+noteID, note.Name)
}

func TestCreateAndUpdateNoteRebuildsTagPayload(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "tag-payload-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "#book/fiction #Work #work #A\u200dB https://example.com/#hidden",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"book", "book/fiction", "Work", "work", "AB"}, note.Tags)

	note, err = ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:    note.Name,
			Content: "#next #A\u200dB",
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"next", "AB"}, note.Tags)

	stored, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
	require.Equal(t, []string{"next", "AB"}, stored.Tags)
}

func TestListNotes(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create userOne
	userOne, err := ts.CreateRegularUser(ctx, "test-user-1")
	require.NoError(t, err)
	require.NotNil(t, userOne)

	// Create userOne context
	userOneCtx := ts.CreateUserContext(ctx, userOne.ID)

	// Create userTwo
	userTwo, err := ts.CreateRegularUser(ctx, "test-user-2")
	require.NoError(t, err)
	require.NotNil(t, userTwo)

	// Create userTwo context
	userTwoCtx := ts.CreateUserContext(ctx, userTwo.ID)

	// Create attachmentOne by userOne
	attachmentOne, err := ts.Service.CreateAttachment(userOneCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{
			Name:     "",
			Filename: "hello.txt",
			Size:     5,
			Type:     "text/plain",
			Content: []byte{
				104, 101, 108, 108, 111,
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, attachmentOne)

	// Create attachmentTwo by userOne
	attachmentTwo, err := ts.Service.CreateAttachment(userOneCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{
			Name:     "",
			Filename: "world.txt",
			Size:     5,
			Type:     "text/plain",
			Content: []byte{
				119, 111, 114, 108, 100,
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, attachmentTwo)

	// Create noteOne with two attachments by userOne
	noteOne, err := ts.Service.CreateNote(userOneCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "Hellooo, any words after this sentence won't be in the snippet. This is the next sentence. And I also have two attachments.",
			Visibility: apipb.Visibility_PROTECTED,
			Attachments: []*apipb.Attachment{
				&apipb.Attachment{
					Name: attachmentOne.Name,
				},
				&apipb.Attachment{
					Name: attachmentTwo.Name,
				},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, noteOne)

	// Create noteTwo by userTwo referencing noteOne
	noteTwo, err := ts.Service.CreateNote(userTwoCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This is a note reminding you to check the attachment attached to noteOne. I have referenced the note below.⬇️",
			Visibility: apipb.Visibility_PROTECTED,
			Relations: []*apipb.NoteRelation{
				&apipb.NoteRelation{
					RelatedNote: &apipb.NoteRelation_NoteRef{
						Name: noteOne.Name,
					},
					Type: apipb.NoteRelation_REFERENCE,
				},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, noteTwo)

	// Create noteThree by userOne
	noteThree, err := ts.Service.CreateNote(userOneCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This is a very popular note. I have 2 reactions!",
			Visibility: apipb.Visibility_PROTECTED,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, noteThree)

	// Create reaction from userOne on noteThree
	reactionOne, err := ts.Service.UpsertNoteReaction(userOneCtx, &apipb.UpsertNoteReactionRequest{
		Parent: noteThree.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "❤️",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, reactionOne)

	// Create reaction from userTwo on noteThree
	reactionTwo, err := ts.Service.UpsertNoteReaction(userTwoCtx, &apipb.UpsertNoteReactionRequest{
		Parent: noteThree.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, reactionTwo)

	notes, err := ts.Service.ListNotes(userOneCtx, &apipb.ListNotesRequest{PageSize: 10})

	require.NoError(t, err)
	require.NotNil(t, notes)
	require.Equal(t, 3, len(notes.Notes))

	// ///////////////
	// VERIFY NOTE ONE
	// ///////////////
	noteOneResIdx := slices.IndexFunc(notes.Notes, func(m *apipb.Note) bool { return m.GetName() == noteOne.GetName() })
	require.NotEqual(t, noteOneResIdx, -1)

	noteOneRes := notes.Notes[noteOneResIdx]
	require.NotNil(t, noteOneRes)

	require.Equal(t, fmt.Sprintf("users/%s", userOne.Username), noteOneRes.GetCreator())
	require.Equal(t, apipb.Visibility_PROTECTED, noteOneRes.GetVisibility())
	require.Equal(t, noteOne.Content, noteOneRes.GetContent())
	require.Equal(t, noteOne.Content[:64]+"...", noteOneRes.GetSnippet(), "noteOne's content is snipped past the 64 char limit")
	require.Len(t, noteOneRes.Attachments, 2)
	require.Len(t, noteOneRes.Relations, 1)
	require.Empty(t, noteOneRes.Reactions)

	// verify noteOne's attachments
	// attachment one
	attachmentOneResIdx := slices.IndexFunc(noteOneRes.Attachments, func(a *apipb.Attachment) bool { return a.GetName() == attachmentOne.GetName() })
	require.NotEqual(t, attachmentOneResIdx, -1)

	attachmentOneRes := noteOneRes.Attachments[attachmentOneResIdx]
	require.NotNil(t, attachmentOneRes)

	require.Equal(t, attachmentOne.GetName(), attachmentOneRes.GetName())
	require.Equal(t, attachmentOne.GetContent(), attachmentOneRes.GetContent())

	// attachment two
	attachmentTwoResIdx := slices.IndexFunc(noteOneRes.Attachments, func(a *apipb.Attachment) bool { return a.GetName() == attachmentTwo.GetName() })
	require.NotEqual(t, attachmentTwoResIdx, -1)

	attachmentTwoRes := noteOneRes.Attachments[attachmentTwoResIdx]
	require.NotNil(t, attachmentTwoRes)
	require.Equal(t, attachmentTwo.GetName(), attachmentTwoRes.GetName())

	require.Equal(t, attachmentTwo.GetName(), attachmentTwoRes.GetName())
	require.Equal(t, attachmentTwo.GetContent(), attachmentTwoRes.GetContent())

	// verify noteOne's relations
	require.Len(t, noteOneRes.Relations, 1)
	noteOneExpectedRelation := &apipb.NoteRelation{
		Note:        &apipb.NoteRelation_NoteRef{Name: noteTwo.GetName()},
		RelatedNote: &apipb.NoteRelation_NoteRef{Name: noteOne.GetName()},
	}
	require.Equal(t, noteOneExpectedRelation.Note.GetName(), noteOneRes.Relations[0].Note.GetName())
	require.Equal(t, noteOneExpectedRelation.RelatedNote.GetName(), noteOneRes.Relations[0].RelatedNote.GetName())

	// ///////////////
	// VERIFY NOTE TWO
	// ///////////////
	noteTwoResIdx := slices.IndexFunc(notes.Notes, func(m *apipb.Note) bool { return m.GetName() == noteTwo.GetName() })
	require.NotEqual(t, noteTwoResIdx, -1)

	noteTwoRes := notes.Notes[noteTwoResIdx]
	require.NotNil(t, noteTwoRes)

	require.Equal(t, fmt.Sprintf("users/%s", userTwo.Username), noteTwoRes.GetCreator())
	require.Equal(t, apipb.Visibility_PROTECTED, noteTwoRes.GetVisibility())
	require.Equal(t, noteTwo.Content, noteTwoRes.GetContent())
	require.Empty(t, noteTwoRes.Attachments)
	require.Len(t, noteTwoRes.Relations, 1)
	require.Empty(t, noteTwoRes.Reactions)

	// verify noteTwo's relations
	require.Len(t, noteTwoRes.Relations, 1)
	noteTwoExpectedRelation := &apipb.NoteRelation{
		Note:        &apipb.NoteRelation_NoteRef{Name: noteTwo.GetName()},
		RelatedNote: &apipb.NoteRelation_NoteRef{Name: noteOne.GetName()},
	}
	require.Equal(t, noteTwoExpectedRelation.Note.GetName(), noteTwoRes.Relations[0].Note.GetName())
	require.Equal(t, noteTwoExpectedRelation.RelatedNote.GetName(), noteTwoRes.Relations[0].RelatedNote.GetName())

	// ///////////////
	// VERIFY NOTE THREE
	// ///////////////
	noteThreeResIdx := slices.IndexFunc(notes.Notes, func(m *apipb.Note) bool { return m.GetName() == noteThree.GetName() })
	require.NotEqual(t, noteThreeResIdx, -1)

	noteThreeRes := notes.Notes[noteThreeResIdx]
	require.NotNil(t, noteThreeRes)

	require.Equal(t, fmt.Sprintf("users/%s", userOne.Username), noteThreeRes.GetCreator())
	require.Equal(t, apipb.Visibility_PROTECTED, noteThreeRes.GetVisibility())
	require.Equal(t, noteThree.Content, noteThreeRes.GetContent())
	require.Empty(t, noteThreeRes.Attachments)
	require.Empty(t, noteThreeRes.Relations)
	require.Len(t, noteThreeRes.Reactions, 2)

	// verify noteThree's reactions
	require.Len(t, noteThreeRes.Reactions, 2)
	// userOne's reaction
	userOneReactionIdx := slices.IndexFunc(noteThreeRes.Reactions, func(r *apipb.Reaction) bool { return r.GetCreator() == fmt.Sprintf("users/%s", userOne.Username) })
	require.NotEqual(t, userOneReactionIdx, -1)

	userOneReaction := noteThreeRes.Reactions[userOneReactionIdx]
	require.NotNil(t, userOneReaction)
	require.Equal(t, "❤️", userOneReaction.ReactionType)

	// userTwo's reaction
	userTwoReactionIdx := slices.IndexFunc(noteThreeRes.Reactions, func(r *apipb.Reaction) bool { return r.GetCreator() == fmt.Sprintf("users/%s", userTwo.Username) })
	require.NotEqual(t, userTwoReactionIdx, -1)

	userTwoReaction := noteThreeRes.Reactions[userTwoReactionIdx]
	require.NotNil(t, userTwoReaction)
	require.Equal(t, "👍", userTwoReaction.ReactionType)
}

func TestListNotesTimeOrderBy(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "time-order-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	noteEarlyCreateLateUpdate, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "early create late update",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
			UpdateTime: timestamppb.New(time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)),
		},
	})
	require.NoError(t, err)
	noteMiddleCreateEarlyUpdate, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "middle create early update",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)),
			UpdateTime: timestamppb.New(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
	})
	require.NoError(t, err)
	noteLateCreateMiddleUpdate, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "late create middle update",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)),
			UpdateTime: timestamppb.New(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)),
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		orderBy   string
		wantNames []string
	}{
		{
			name:    "default create time",
			orderBy: "",
			wantNames: []string{
				noteLateCreateMiddleUpdate.Name,
				noteMiddleCreateEarlyUpdate.Name,
				noteEarlyCreateLateUpdate.Name,
			},
		},
		{
			name:    "explicit create time",
			orderBy: "create_time desc",
			wantNames: []string{
				noteLateCreateMiddleUpdate.Name,
				noteMiddleCreateEarlyUpdate.Name,
				noteEarlyCreateLateUpdate.Name,
			},
		},
		{
			name:    "explicit update time",
			orderBy: "update_time desc",
			wantNames: []string{
				noteEarlyCreateLateUpdate.Name,
				noteLateCreateMiddleUpdate.Name,
				noteMiddleCreateEarlyUpdate.Name,
			},
		},
		{
			name:    "pinned with explicit create time",
			orderBy: "pinned desc, create_time desc",
			wantNames: []string{
				noteLateCreateMiddleUpdate.Name,
				noteMiddleCreateEarlyUpdate.Name,
				noteEarlyCreateLateUpdate.Name,
			},
		},
		{
			name:    "explicit create time ascending",
			orderBy: "create_time asc",
			wantNames: []string{
				noteEarlyCreateLateUpdate.Name,
				noteMiddleCreateEarlyUpdate.Name,
				noteLateCreateMiddleUpdate.Name,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := ts.Service.ListNotes(userCtx, &apipb.ListNotesRequest{
				PageSize: 10,
				OrderBy:  test.orderBy,
			})
			require.NoError(t, err)
			require.Len(t, resp.Notes, len(test.wantNames))

			gotNames := make([]string, 0, len(resp.Notes))
			for _, note := range resp.Notes {
				gotNames = append(gotNames, note.Name)
			}
			require.Equal(t, test.wantNames, gotNames)
		})
	}

	_, err = ts.Service.ListNotes(userCtx, &apipb.ListNotesRequest{
		PageSize: 10,
		OrderBy:  "display_time desc",
	})
	require.Error(t, err)
}

func TestListNotesSkipsReactionsWithMissingCreators(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "note-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	reactor, err := ts.CreateRegularUser(ctx, "note-reactor")
	require.NoError(t, err)
	reactorCtx := ts.CreateUserContext(ctx, reactor.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with orphan reaction",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.UpsertNoteReaction(reactorCtx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: reactor.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListNotes(ownerCtx, &apipb.ListNotesRequest{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, resp.Notes, 1)
	require.Equal(t, note.Name, resp.Notes[0].Name)
	require.Empty(t, resp.Notes[0].Reactions)
}

func TestListNotesSkipsNotesWithMissingCreators(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "note-visible-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	orphanCreator, err := ts.CreateRegularUser(ctx, "note-orphan-creator")
	require.NoError(t, err)
	orphanCtx := ts.CreateUserContext(ctx, orphanCreator.ID)

	ownerNote, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "owner note",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNote(orphanCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "orphan note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: orphanCreator.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListNotes(ownerCtx, &apipb.ListNotesRequest{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, resp.Notes, 1)
	require.Equal(t, ownerNote.Name, resp.Notes[0].Name)
}

func TestListNoteCommentsSkipsCommentsWithMissingCreators(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	commenter, err := ts.CreateRegularUser(ctx, "comment-orphan")
	require.NoError(t, err)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with comment",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(commenterCtx, &apipb.CreateNoteCommentRequest{
		Parent: note.Name,
		Comment: &apipb.Note{
			Content:    "comment to orphan",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: commenter.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Empty(t, resp.Comments)
}

func TestCreateNoteCommentRejectsSelfReference(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "comment-self-reference-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "comment context", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent:    note.Name,
		CommentId: note.Name[len("notes/"):],
		Comment:   &apipb.Note{Content: "self reference", Visibility: apipb.Visibility_PUBLIC},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	comments, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Empty(t, comments.Comments)
}

func TestUpdateNotePlacementAndAudienceContract(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	sourceAdmin, err := ts.CreateRegularUser(ctx, "placement-source-admin")
	require.NoError(t, err)
	author, err := ts.CreateRegularUser(ctx, "placement-author")
	require.NoError(t, err)
	sourceAdminCtx := ts.CreateUserContext(ctx, sourceAdmin.ID)
	authorCtx := ts.CreateUserContext(ctx, author.ID)

	source, err := ts.Service.CreateSpace(sourceAdminCtx, &apipb.CreateSpaceRequest{
		SpaceId: "placement-source",
		Space:   &apipb.Space{Title: "Placement Source"},
	})
	require.NoError(t, err)
	target, err := ts.Service.CreateSpace(authorCtx, &apipb.CreateSpaceRequest{
		SpaceId: "placement-target",
		Space:   &apipb.Space{Title: "Placement Target"},
	})
	require.NoError(t, err)
	invitation, err := ts.Service.CreateSpaceInvitation(sourceAdminCtx, &apipb.CreateSpaceInvitationRequest{
		Parent: source.Name,
		SpaceInvitation: &apipb.SpaceInvitation{
			Invitee: "users/" + author.Username,
			Role:    apipb.SpaceMember_USER,
		},
	})
	require.NoError(t, err)
	_, err = ts.Service.AcceptSpaceInvitation(authorCtx, &apipb.AcceptSpaceInvitationRequest{Name: invitation.Name})
	require.NoError(t, err)

	protectedNote, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "assign without audience change", Visibility: apipb.Visibility_PROTECTED,
	}})
	require.NoError(t, err)
	sourceName := source.Name
	protectedNote, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: protectedNote.Name, Space: &sourceName},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.NoError(t, err)
	require.Equal(t, source.Name, protectedNote.GetSpace())
	require.Equal(t, apipb.Visibility_PROTECTED, protectedNote.Visibility, "assigning must preserve the note audience")

	_, err = ts.Service.UpdateNote(sourceAdminCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: protectedNote.Name},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "Space administrators cannot change another author's note placement")
	protectedNote, err = ts.Service.GetNote(authorCtx, &apipb.GetNoteRequest{Name: protectedNote.Name})
	require.NoError(t, err)
	require.Equal(t, source.Name, protectedNote.GetSpace())

	unassigned, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "unassigned", Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	_, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: unassigned.Name, Visibility: apipb.Visibility_SPACE},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"visibility"}},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "SPACE visibility requires an assigned Space")

	spaceMembersNote, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "members audience", Visibility: apipb.Visibility_SPACE, Space: &sourceName,
	}})
	require.NoError(t, err)
	targetName := target.Name
	_, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: spaceMembersNote.Name, Space: &targetName},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "moving SPACE visibility requires an explicit audience confirmation")
	_, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: spaceMembersNote.Name},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "unassigning SPACE visibility requires a replacement audience")
	spaceMembersNote, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: spaceMembersNote.Name, Visibility: apipb.Visibility_PRIVATE},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space", "visibility"}},
	})
	require.NoError(t, err)
	require.Nil(t, spaceMembersNote.Space)
	require.Equal(t, apipb.Visibility_PRIVATE, spaceMembersNote.Visibility)

	moveNote, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "move original", Visibility: apipb.Visibility_PUBLIC, Space: &sourceName,
	}})
	require.NoError(t, err)
	withdrawNote, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "withdraw original", Visibility: apipb.Visibility_PUBLIC, Space: &sourceName,
	}})
	require.NoError(t, err)
	membersOnlyNote, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "members only", Visibility: apipb.Visibility_SPACE, Space: &sourceName,
	}})
	require.NoError(t, err)
	_, err = ts.Service.DeleteSpaceMember(sourceAdminCtx, &apipb.DeleteSpaceMemberRequest{
		Name: source.Name + "/members/" + author.Username,
	})
	require.NoError(t, err)
	_, err = ts.Service.GetNote(authorCtx, &apipb.GetNoteRequest{Name: membersOnlyNote.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "the author loses SPACE audience read access after leaving the Space")

	_, err = ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: moveNote.Name, Space: &targetName, Content: "smuggled content"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space", "content"}},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "a removed author cannot smuggle content into a lifecycle move")
	unchangedMove, err := ts.Service.GetNote(authorCtx, &apipb.GetNoteRequest{Name: moveNote.Name})
	require.NoError(t, err)
	require.Equal(t, "move original", unchangedMove.Content)
	require.Equal(t, source.Name, unchangedMove.GetSpace())

	moved, err := ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: moveNote.Name, Space: &targetName},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.NoError(t, err)
	require.Equal(t, target.Name, moved.GetSpace())
	require.Equal(t, "move original", moved.Content)
	require.Equal(t, apipb.Visibility_PUBLIC, moved.Visibility)

	withdrawn, err := ts.Service.UpdateNote(authorCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: withdrawNote.Name},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space"}},
	})
	require.NoError(t, err)
	require.Nil(t, withdrawn.Space)
	require.Equal(t, "withdraw original", withdrawn.Content)
	require.Equal(t, apipb.Visibility_PUBLIC, withdrawn.Visibility)
}

func TestListNoteCommentsPaginates(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "comment-page-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "note with paged comments",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	for i := range 3 {
		_, err = ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
			Parent: note.Name,
			Comment: &apipb.Note{
				Content:    fmt.Sprintf("comment %d", i),
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)
	}

	firstPage, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, firstPage.Comments, 2)
	require.NotEmpty(t, firstPage.NextPageToken)

	secondPage, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name, PageToken: firstPage.NextPageToken})
	require.NoError(t, err)
	require.Len(t, secondPage.Comments, 1)
	require.Empty(t, secondPage.NextPageToken)
}

func TestListNoteCommentsFiltersArchivedBeforePagination(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "comment-archive-page-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "note with archived comments",
		Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)

	comments := make([]*apipb.Note, 0, 5)
	for i := range 5 {
		comment, err := ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
			Parent:  note.Name,
			Comment: &apipb.Note{Content: fmt.Sprintf("comment %d", i)},
		})
		require.NoError(t, err)
		comments = append(comments, comment)
	}
	for _, comment := range comments[3:] {
		_, err := ts.Service.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
			Note:       &apipb.Note{Name: comment.Name, State: apipb.State_ARCHIVED},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
		})
		require.NoError(t, err)
	}

	firstPage, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, firstPage.Comments, 2)
	require.NotEmpty(t, firstPage.NextPageToken)
	secondPage, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: note.Name, PageToken: firstPage.NextPageToken})
	require.NoError(t, err)
	require.Len(t, secondPage.Comments, 1)
	require.Empty(t, secondPage.NextPageToken)
}

func TestNoteCommentKeepsIndependentVisibilityAndAllowsAudienceUpdates(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "private-comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	parent, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "private parent",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	comment, err := ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent: parent.Name,
		Comment: &apipb.Note{
			Content:    "client requested public comment",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	require.Equal(t, apipb.Visibility_PUBLIC, comment.Visibility)

	_, err = ts.Service.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:       comment.Name,
			Visibility: apipb.Visibility_PROTECTED,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"visibility"}},
	})
	require.NoError(t, err)

	updatedComment, err := ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: comment.Name})
	require.NoError(t, err)
	require.Equal(t, apipb.Visibility_PROTECTED, updatedComment.Visibility)

	unchangedParent, err := ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: parent.Name})
	require.NoError(t, err)
	require.Equal(t, apipb.Visibility_PRIVATE, unchangedParent.Visibility)
}

func TestCreateNoteCommentDoesNotRevealArchivedPrivateNote(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "archived-comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	other, err := ts.CreateRegularUser(ctx, "archived-comment-other")
	require.NoError(t, err)
	otherCtx := ts.CreateUserContext(ctx, other.ID)
	parent, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "archived private parent",
		Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	_, err = ts.Service.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: parent.Name, State: apipb.State_ARCHIVED},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	require.NoError(t, err)

	_, err = ts.Service.CreateNoteComment(otherCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "should not reveal parent state"},
	})
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "owner still cannot comment"},
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestGetNoteCommentUsesNoteLocalReadAccessAndConcealsContext(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "legacy-comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	other, err := ts.CreateRegularUser(ctx, "legacy-comment-other")
	require.NoError(t, err)
	otherCtx := ts.CreateUserContext(ctx, other.ID)

	parent, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "private parent for legacy comment",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	createdComment, err := ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent: parent.Name,
		Comment: &apipb.Note{
			Content:    "public comment with private context",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	commentName := createdComment.Name
	publicComment, err := ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: commentName})
	require.NoError(t, err)
	require.Equal(t, parent.Name, publicComment.GetParent(), "parent identity remains available without granting access")

	otherComment, err := ts.Service.GetNote(otherCtx, &apipb.GetNoteRequest{Name: commentName})
	require.NoError(t, err)
	require.Equal(t, parent.Name, otherComment.GetParent(), "parent identity is independent of parent readability")

	comment, err := ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: commentName})
	require.NoError(t, err)
	require.Equal(t, parent.Name, comment.GetParent())

	_, err = ts.Service.ListNoteComments(ctx, &apipb.ListNoteCommentsRequest{Parent: parent.Name})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = ts.Service.ListNoteComments(otherCtx, &apipb.ListNoteCommentsRequest{Parent: parent.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	comments, err := ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: parent.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1)
	require.Equal(t, commentName, comments.Comments[0].Name)
}

func TestAssignedNoteCommentListingUsesNoteLocalReadAccess(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "assigned-comment-owner")
	require.NoError(t, err)
	member, err := ts.CreateRegularUser(ctx, "assigned-comment-member")
	require.NoError(t, err)
	outsider, err := ts.CreateRegularUser(ctx, "assigned-comment-outsider")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	memberCtx := ts.CreateUserContext(ctx, member.ID)
	outsiderCtx := ts.CreateUserContext(ctx, outsider.ID)

	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "assigned-comment-space", Title: "Assigned comments"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  member.ID,
		Role:    store.SpaceMemberRoleAdmin,
	}, owner.ID)
	require.NoError(t, err)
	contextNote, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "assigned public comment context",
		Visibility: apipb.Visibility_PUBLIC,
		Space:      new("spaces/" + space.UID),
	}})
	require.NoError(t, err)
	comment, err := ts.Service.CreateNoteComment(memberCtx, &apipb.CreateNoteCommentRequest{
		Parent: contextNote.Name,
		Comment: &apipb.Note{
			Content:    "public independently readable comment",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	comments, err := ts.Service.ListNoteComments(memberCtx, &apipb.ListNoteCommentsRequest{Parent: contextNote.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1)
	require.Equal(t, comment.Name, comments.Comments[0].Name)

	comments, err = ts.Service.ListNoteComments(ctx, &apipb.ListNoteCommentsRequest{Parent: contextNote.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1, "anonymous access follows the PUBLIC context and comment audiences")
	comments, err = ts.Service.ListNoteComments(outsiderCtx, &apipb.ListNoteCommentsRequest{Parent: contextNote.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1, "Space placement adds no read gate for a signed-in non-member")
	directComment, err := ts.Service.GetNote(outsiderCtx, &apipb.GetNoteRequest{Name: comment.Name})
	require.NoError(t, err, "the comment note remains independently readable")
	require.Equal(t, contextNote.Name, directComment.GetParent(), "two-endpoint relation projection remains note-local")

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, owner.ID))
	comments, err = ts.Service.ListNoteComments(ownerCtx, &apipb.ListNoteCommentsRequest{Parent: contextNote.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1, "the removed author still reads PUBLIC notes through their audience")
}

func TestNoteCommentDoesNotFollowParentVisibilityChanges(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "dynamic-comment-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	parent, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "initially private",
		Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	comment, err := ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "independently private"},
	})
	require.NoError(t, err)
	require.Equal(t, apipb.Visibility_PRIVATE, comment.Visibility)

	_, err = ts.Service.UpdateNote(ownerCtx, &apipb.UpdateNoteRequest{
		Note:       &apipb.Note{Name: parent.Name, Visibility: apipb.Visibility_PUBLIC},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"visibility"}},
	})
	require.NoError(t, err)

	_, err = ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: comment.Name})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	comments, err := ts.Service.ListNoteComments(ctx, &apipb.ListNoteCommentsRequest{Parent: parent.Name})
	require.NoError(t, err)
	require.Empty(t, comments.Comments, "context visibility must not expand a private comment")
}

func TestGlobalFeedExcludesCommentsAndDeletesThemWithContext(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "comment-feed-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	contextNote, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "context", Visibility: apipb.Visibility_PUBLIC,
	}})
	require.NoError(t, err)
	comment, err := ts.Service.CreateNoteComment(ownerCtx, &apipb.CreateNoteCommentRequest{
		Parent: contextNote.Name, Comment: &apipb.Note{Content: "independent comment", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	feed, err := ts.Service.ListNotes(ownerCtx, &apipb.ListNotesRequest{PageSize: 20})
	require.NoError(t, err)
	require.Len(t, feed.Notes, 1)
	require.Equal(t, contextNote.Name, feed.Notes[0].Name)

	_, err = ts.Service.DeleteNote(ownerCtx, &apipb.DeleteNoteRequest{Name: contextNote.Name})
	require.NoError(t, err)
	_, err = ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: comment.Name})
	require.Equal(t, codes.NotFound, status.Code(err))

	feed, err = ts.Service.ListNotes(ownerCtx, &apipb.ListNotesRequest{PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, feed.Notes)
}

// TestCreateNoteWithCustomTimestamps tests that custom timestamps can be set when creating notes and comments.
// This addresses issue #5483: https://github.com/anangnovriadi/roster-tracker/issues/5483
func TestCreateNoteWithCustomTimestamps(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateRegularUser(ctx, "test-user-timestamps")
	require.NoError(t, err)
	require.NotNil(t, user)

	userCtx := ts.CreateUserContext(ctx, user.ID)

	// Define custom timestamps (January 1, 2020)
	customCreateTime := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	customUpdateTime := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)

	// Test 1: Create a note with custom create_time
	noteWithCreateTime, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This note has a custom creation time",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(customCreateTime),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, noteWithCreateTime)
	require.Equal(t, customCreateTime.Unix(), noteWithCreateTime.CreateTime.AsTime().Unix(), "create_time should match the custom timestamp")

	// Test 2: Create a note with custom update_time
	noteWithUpdateTime, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This note has a custom update time",
			Visibility: apipb.Visibility_PRIVATE,
			UpdateTime: timestamppb.New(customUpdateTime),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, noteWithUpdateTime)
	require.Equal(t, customUpdateTime.Unix(), noteWithUpdateTime.UpdateTime.AsTime().Unix(), "update_time should match the custom timestamp")

	// Test 3: Create a note with all custom timestamps
	noteWithAllTimestamps, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This note has all custom timestamps",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(customCreateTime),
			UpdateTime: timestamppb.New(customUpdateTime),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, noteWithAllTimestamps)
	require.Equal(t, customCreateTime.Unix(), noteWithAllTimestamps.CreateTime.AsTime().Unix(), "create_time should match the custom timestamp")
	require.Equal(t, customUpdateTime.Unix(), noteWithAllTimestamps.UpdateTime.AsTime().Unix(), "update_time should match the custom timestamp")

	// Test 4: Create a comment (note relation) with custom timestamps
	parentNote, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This is the parent note",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, parentNote)

	customCommentCreateTime := time.Date(2021, 6, 15, 10, 30, 0, 0, time.UTC)
	comment, err := ts.Service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
		Parent: parentNote.Name,
		Comment: &apipb.Note{
			Content:    "This is a comment with custom create time",
			Visibility: apipb.Visibility_PRIVATE,
			CreateTime: timestamppb.New(customCommentCreateTime),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, comment)
	require.Equal(t, customCommentCreateTime.Unix(), comment.CreateTime.AsTime().Unix(), "comment create_time should match the custom timestamp")

	// Test 5: Verify that notes without custom timestamps still get auto-generated ones
	noteWithoutTimestamps, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "This note has auto-generated timestamps",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, noteWithoutTimestamps)
	require.NotNil(t, noteWithoutTimestamps.CreateTime, "create_time should be auto-generated")
	require.NotNil(t, noteWithoutTimestamps.UpdateTime, "update_time should be auto-generated")
	require.True(t, time.Now().Unix()-noteWithoutTimestamps.CreateTime.AsTime().Unix() < 5, "create_time should be recent (within 5 seconds)")
}

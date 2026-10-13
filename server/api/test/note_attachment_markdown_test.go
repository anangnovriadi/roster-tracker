package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/server/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteManagedAttachmentImages(t *testing.T) {
	ctx := context.Background()

	t.Run("create accepts canonical and legacy managed image URLs", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-create")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		first := createTestImageAttachment(userCtx, t, ts, "first.png")
		second := createTestImageAttachment(userCtx, t, ts, "second.png")
		firstUID := strings.TrimPrefix(first.Name, "attachments/")
		secondUID := strings.TrimPrefix(second.Name, "attachments/")

		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content: fmt.Sprintf("![first](/file/attachments/%s)\n![second](/file/attachments/%s/second.png)", firstUID, secondUID),
			Attachments: []*apipb.Attachment{
				{Name: first.Name},
				{Name: second.Name},
			},
			Visibility: apipb.Visibility_PRIVATE,
		}})
		require.NoError(t, err)
		require.Len(t, note.Attachments, 2)
	})

	t.Run("create validates references before creating the note", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-missing")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
			NoteId: "managed-missing-note",
			Note: &apipb.Note{
				Content:    "![missing](http://localhost:8080/file/attachments/missing-image)",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		uid := "managed-missing-note"
		stored, getErr := ts.Store.GetNote(ctx, &store.FindNote{UID: &uid})
		require.NoError(t, getErr)
		require.Nil(t, stored)
	})

	t.Run("managed image must have an image MIME type", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-mime")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		textAttachment, err := ts.Service.CreateAttachment(userCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
			Filename: "not-image.txt",
			Type:     "text/plain",
			Content:  []byte("not an image"),
		}})
		require.NoError(t, err)
		uid := strings.TrimPrefix(textAttachment.Name, "attachments/")

		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![wrong type](/file/attachments/%s)", uid),
			Attachments: []*apipb.Attachment{{Name: textAttachment.Name}},
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("malformed managed URLs are rejected", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-malformed")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "query.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")

		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![query](/file/attachments/%s?share_token=secret)", uid),
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("protocol-relative managed URLs are rejected", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-network-path")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "network-path.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")

		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![network path](//localhost:8080/file/attachments/%s)", uid),
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("cross-origin protocol-relative image URLs remain external", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "external-network-path")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content: "![external](//cdn.example.com/file/attachments/external-image)",
		}})
		require.NoError(t, err)
		require.Empty(t, note.Attachments)
	})

	t.Run("same-origin absolute managed URL is verified and accepted", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Profile.InstanceURL = "http://localhost"
		user, err := ts.CreateRegularUser(ctx, "managed-absolute")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "absolute.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")

		content := fmt.Sprintf("![absolute](http://localhost:80/file/attachments/%s)", uid)
		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: content}})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))

		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     content,
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.NoError(t, err)
		require.Len(t, note.Attachments, 1)
	})

	t.Run("content validation sees attachments beyond the default page", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-many-attachments")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: "many attachments"}})
		require.NoError(t, err)
		noteID := noteIDFromName(ctx, t, ts, note.Name)

		referenced, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
			UID: shortuuid.New(), CreatorID: user.ID, Filename: "referenced.png", Type: "image/png", NoteID: &noteID,
		})
		require.NoError(t, err)
		for i := range 100 {
			attachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
				UID: shortuuid.New(), CreatorID: user.ID, Filename: fmt.Sprintf("extra-%03d.png", i), Type: "image/png", NoteID: &noteID,
			})
			require.NoError(t, err)
			updatedTs := referenced.UpdatedTs + int64(i) + 1
			require.NoError(t, ts.Store.UpdateAttachment(ctx, &store.UpdateAttachment{ID: attachment.ID, UpdatedTs: &updatedTs}))
		}

		updated, err := ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
			Note: &apipb.Note{
				Name:    note.Name,
				Content: fmt.Sprintf("![oldest](/file/attachments/%s)", referenced.UID),
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
		})
		require.NoError(t, err)
		require.Len(t, updated.Attachments, 101)
	})

	t.Run("set attachments cannot remove an image referenced by content", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-set")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "kept.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![kept](/file/attachments/%s)", uid),
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.NoError(t, err)

		_, err = ts.Service.SetNoteAttachments(userCtx, &apipb.SetNoteAttachmentsRequest{Name: note.Name})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		attachments, listErr := ts.Service.ListNoteAttachments(userCtx, &apipb.ListNoteAttachmentsRequest{Parent: note.Name})
		require.NoError(t, listErr)
		require.Len(t, attachments.Attachments, 1)
	})

	t.Run("content and attachments are validated as one final update state", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-update")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "removed.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![removed](/file/attachments/%s)", uid),
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.NoError(t, err)

		updated, err := ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
			Note:       &apipb.Note{Name: note.Name, Content: "image removed", Attachments: []*apipb.Attachment{}},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"attachments", "content"}},
		})
		require.NoError(t, err)
		require.Equal(t, "image removed", updated.Content)
		require.Empty(t, updated.Attachments)
		stored, getErr := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
		require.NoError(t, getErr)
		require.Nil(t, stored)
	})

	t.Run("local deletion failure reports committed row deletion", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-delete-retry")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "retry.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     "attachment will be removed",
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.NoError(t, err)
		storedImage, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
		require.NoError(t, err)
		require.NotNil(t, storedImage)
		localReference := "retry-delete.png"
		localPath := filepath.Join(ts.Profile.Data, localReference)
		require.NoError(t, os.WriteFile(localPath, []byte("local image"), 0o600))
		_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx,
			"UPDATE attachment SET storage_type = ?, reference = ? WHERE id = ?",
			"LOCAL", localReference, storedImage.ID,
		)
		require.NoError(t, err)
		_, err = ts.Service.UpdateNote(store.WithDeleteAttachmentStorageFailpoint(userCtx), &apipb.UpdateNoteRequest{
			Note:       &apipb.Note{Name: note.Name, Content: "attachment removed", Attachments: []*apipb.Attachment{}},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content", "attachments"}},
		})
		require.Equal(t, codes.Internal, status.Code(err))
		require.ErrorContains(t, err, "note was updated but attachment storage cleanup failed")
		require.ErrorContains(t, err, store.ErrDeleteAttachmentStorageFailpoint.Error())
		storedNote, getErr := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: note.Name})
		require.NoError(t, getErr)
		require.Equal(t, "attachment removed", storedNote.Content)
		storedImage, getErr = ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
		require.NoError(t, getErr)
		require.Nil(t, storedImage)
		_, statErr := os.Stat(localPath)
		require.NoError(t, statErr)
	})

	t.Run("failed content validation does not mutate the note", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-update-failure")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: "original"}})
		require.NoError(t, err)

		_, err = ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
			Note:       &apipb.Note{Name: note.Name, Content: "![missing](/file/attachments/missing-update-image)"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		stored, getErr := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: note.Name})
		require.NoError(t, getErr)
		require.Equal(t, "original", stored.Content)
	})

	t.Run("delete APIs reject referenced attachments", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "managed-delete")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		image := createTestImageAttachment(userCtx, t, ts, "referenced.png")
		uid := strings.TrimPrefix(image.Name, "attachments/")
		_, err = ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
			Content:     fmt.Sprintf("![referenced](/file/attachments/%s)", uid),
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		}})
		require.NoError(t, err)

		_, err = ts.Service.DeleteAttachment(userCtx, &apipb.DeleteAttachmentRequest{Name: image.Name})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		_, err = ts.Service.BatchDeleteAttachments(userCtx, &apipb.BatchDeleteAttachmentsRequest{Names: []string{image.Name}})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		stored, getErr := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
		require.NoError(t, getErr)
		require.NotNil(t, stored)
	})
}

func TestNoteAttachmentBindingDoesNotReparent(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	user, err := ts.CreateRegularUser(ctx, "no-reparent")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	image := createTestImageAttachment(userCtx, t, ts, "bound.png")
	first, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Attachments: []*apipb.Attachment{{Name: image.Name}}}})
	require.NoError(t, err)
	second, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: "second"}})
	require.NoError(t, err)

	_, err = ts.Service.SetNoteAttachments(userCtx, &apipb.SetNoteAttachmentsRequest{
		Name:        second.Name,
		Attachments: []*apipb.Attachment{{Name: image.Name}},
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	uid, err := api.ExtractAttachmentUIDFromName(image.Name)
	require.NoError(t, err)
	stored, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, stored.NoteID)
	require.Equal(t, noteIDFromName(ctx, t, ts, first.Name), *stored.NoteID)
}

func TestCreateNoteDoesNotBindAnotherUsersAttachmentOrCreatePartialNote(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	owner, err := ts.CreateRegularUser(ctx, "foreign-attachment-owner")
	require.NoError(t, err)
	other, err := ts.CreateRegularUser(ctx, "foreign-attachment-caller")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	otherCtx := ts.CreateUserContext(ctx, other.ID)
	image := createTestImageAttachment(ownerCtx, t, ts, "foreign.png")

	_, err = ts.Service.CreateNote(otherCtx, &apipb.CreateNoteRequest{
		NoteId: "no-partial-foreign-note",
		Note: &apipb.Note{
			Content:     "foreign attachment",
			Attachments: []*apipb.Attachment{{Name: image.Name}},
		},
	})
	require.Equal(t, codes.NotFound, status.Code(err))
	uid := "no-partial-foreign-note"
	note, getErr := ts.Store.GetNote(ctx, &store.FindNote{UID: &uid})
	require.NoError(t, getErr)
	require.Nil(t, note)
	attachmentUID := strings.TrimPrefix(image.Name, "attachments/")
	attachment, getErr := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &attachmentUID})
	require.NoError(t, getErr)
	require.NotNil(t, attachment)
	require.Nil(t, attachment.NoteID)
}

func createTestImageAttachment(ctx context.Context, t *testing.T, ts *TestService, filename string) *apipb.Attachment {
	t.Helper()
	attachment, err := ts.Service.CreateAttachment(ctx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: filename,
		Type:     "image/png",
		Content:  []byte("test image"),
	}})
	require.NoError(t, err)
	return attachment
}

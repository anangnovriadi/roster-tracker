package api

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteCreateSharedPreparationPreservesFields(t *testing.T) {
	ctx := context.Background()
	service := newIntegrationService(t)
	user := createSpaceTestUser(ctx, t, service, "note-create-fields", store.RoleUser)
	userCtx := userCtx(ctx, user.ID)

	referenceTarget, err := service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		NoteId: "create-fields-reference",
		Note:   &apipb.Note{Content: "reference target", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)

	createAttachment := func(id string) *apipb.Attachment {
		t.Helper()
		attachment, err := service.CreateAttachment(userCtx, &apipb.CreateAttachmentRequest{
			AttachmentId: id,
			Attachment: &apipb.Attachment{
				Filename: id + ".png",
				Type:     "image/png",
				Content:  []byte("test image"),
			},
		})
		require.NoError(t, err)
		return attachment
	}
	createReference := func() *apipb.NoteRelation {
		return &apipb.NoteRelation{
			RelatedNote: &apipb.NoteRelation_NoteRef{Name: referenceTarget.Name},
			Type:        apipb.NoteRelation_REFERENCE,
		}
	}

	topAttachment := createAttachment("create-fields-top-image")
	topAttachmentUID := strings.TrimPrefix(topAttachment.Name, "attachments/")
	topCreateTime := time.Date(2022, time.March, 4, 5, 6, 7, 0, time.UTC)
	topUpdateTime := topCreateTime.Add(2 * time.Hour)
	topLocation := &apipb.Location{Placeholder: "top location", Latitude: 1.25, Longitude: 2.5}
	topNote, err := service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		NoteId: "create-fields-top",
		Note: &apipb.Note{
			Content:     fmt.Sprintf("![top](/file/attachments/%s)", topAttachmentUID),
			Visibility:  apipb.Visibility_PRIVATE,
			CreateTime:  timestamppb.New(topCreateTime),
			UpdateTime:  timestamppb.New(topUpdateTime),
			Attachments: []*apipb.Attachment{{Name: topAttachment.Name}},
			Relations:   []*apipb.NoteRelation{createReference()},
			Location:    topLocation,
		},
	})
	require.NoError(t, err)
	require.Equal(t, topCreateTime.Unix(), topNote.CreateTime.AsTime().Unix())
	require.Equal(t, topUpdateTime.Unix(), topNote.UpdateTime.AsTime().Unix())
	require.Equal(t, topLocation.Placeholder, topNote.Location.Placeholder)
	require.Equal(t, topLocation.Latitude, topNote.Location.Latitude)
	require.Equal(t, topLocation.Longitude, topNote.Location.Longitude)
	require.Len(t, topNote.Attachments, 1)
	require.Equal(t, topAttachment.Name, topNote.Attachments[0].Name)
	requireNoteCreateRelation(t, topNote, apipb.NoteRelation_REFERENCE, referenceTarget.Name)

	commentAttachment := createAttachment("create-fields-comment-image")
	commentAttachmentUID := strings.TrimPrefix(commentAttachment.Name, "attachments/")
	commentCreateTime := time.Date(2023, time.April, 5, 6, 7, 8, 0, time.UTC)
	commentUpdateTime := commentCreateTime.Add(3 * time.Hour)
	commentLocation := &apipb.Location{Placeholder: "comment location", Latitude: 3.75, Longitude: 4.5}
	comment, err := service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
		Parent:    topNote.Name,
		CommentId: "create-fields-comment",
		Comment: &apipb.Note{
			Content:     fmt.Sprintf("![comment](/file/attachments/%s)", commentAttachmentUID),
			Visibility:  apipb.Visibility_PROTECTED,
			CreateTime:  timestamppb.New(commentCreateTime),
			UpdateTime:  timestamppb.New(commentUpdateTime),
			Attachments: []*apipb.Attachment{{Name: commentAttachment.Name}},
			Relations:   []*apipb.NoteRelation{createReference()},
			Location:    commentLocation,
		},
	})
	require.NoError(t, err)
	require.Equal(t, commentCreateTime.Unix(), comment.CreateTime.AsTime().Unix())
	require.Equal(t, commentUpdateTime.Unix(), comment.UpdateTime.AsTime().Unix())
	require.Equal(t, commentLocation.Placeholder, comment.Location.Placeholder)
	require.Equal(t, commentLocation.Latitude, comment.Location.Latitude)
	require.Equal(t, commentLocation.Longitude, comment.Location.Longitude)
	require.Equal(t, apipb.Visibility_PROTECTED, comment.Visibility)
	require.Len(t, comment.Attachments, 1)
	require.Equal(t, commentAttachment.Name, comment.Attachments[0].Name)
	requireNoteCreateRelation(t, comment, apipb.NoteRelation_COMMENT, topNote.Name)
	requireNoteCreateRelation(t, comment, apipb.NoteRelation_REFERENCE, referenceTarget.Name)
}

func TestNoteCreateDuplicateIDsUseAlreadyExists(t *testing.T) {
	ctx := context.Background()
	service := newIntegrationService(t)
	user := createSpaceTestUser(ctx, t, service, "note-create-duplicates", store.RoleUser)
	userCtx := userCtx(ctx, user.ID)

	parent, err := service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		NoteId: "duplicate-parent",
		Note:   &apipb.Note{Content: "parent", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)
	_, err = service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		NoteId: "duplicate-parent",
		Note:   &apipb.Note{Content: "duplicate", Visibility: apipb.Visibility_PRIVATE},
	})
	require.Equal(t, codes.AlreadyExists, status.Code(err))

	createComment := func() error {
		_, err := service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
			Parent:    parent.Name,
			CommentId: "duplicate-comment",
			Comment:   &apipb.Note{Content: "comment", Visibility: apipb.Visibility_PRIVATE},
		})
		return err
	}
	require.NoError(t, createComment())
	require.Equal(t, codes.AlreadyExists, status.Code(createComment()))
}

func requireNoteCreateRelation(t *testing.T, note *apipb.Note, relationType apipb.NoteRelation_Type, relatedNoteName string) {
	t.Helper()
	for _, relation := range note.Relations {
		if relation.Type == relationType && relation.GetRelatedNote().GetName() == relatedNoteName {
			require.Equal(t, note.Name, relation.GetNote().GetName())
			return
		}
	}
	require.Failf(t, "note relation not found", "missing %s relation from %s to %s", relationType, note.Name, relatedNoteName)
}

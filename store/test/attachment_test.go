package test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestAttachmentNeedsInstanceStorageSetting(t *testing.T) {
	tests := []struct {
		name       string
		attachment *store.Attachment
		want       bool
	}{
		{
			name: "nil attachment",
		},
		{
			name: "local attachment",
			attachment: &store.Attachment{
				StorageType: storepb.AttachmentStorageType_LOCAL,
			},
		},
		{
			name: "s3 attachment without payload",
			attachment: &store.Attachment{
				StorageType: storepb.AttachmentStorageType_S3,
			},
		},
		{
			name: "s3 attachment with embedded config",
			attachment: &store.Attachment{
				StorageType: storepb.AttachmentStorageType_S3,
				Payload: &storepb.AttachmentPayload{
					Payload: &storepb.AttachmentPayload_S3Object_{
						S3Object: &storepb.AttachmentPayload_S3Object{
							S3Config: &storepb.StorageS3Config{},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "s3 attachment without embedded config",
			attachment: &store.Attachment{
				StorageType: storepb.AttachmentStorageType_S3,
				Payload: &storepb.AttachmentPayload{
					Payload: &storepb.AttachmentPayload_S3Object_{
						S3Object: &storepb.AttachmentPayload_S3Object{},
					},
				},
			},
			want: true,
		},
		{
			name: "s3 attachment with storage ID",
			attachment: &store.Attachment{
				StorageType: storepb.AttachmentStorageType_S3,
				Payload: &storepb.AttachmentPayload{
					Payload: &storepb.AttachmentPayload_S3Object_{
						S3Object: &storepb.AttachmentPayload_S3Object{StorageId: "s3-primary"},
					},
				},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := store.AttachmentNeedsInstanceStorageSetting(test.attachment); got != test.want {
				t.Fatalf("AttachmentNeedsInstanceStorageSetting() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAttachmentStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	_, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: 101,
		Filename:  "test.epub",
		Blob:      []byte("test"),
		Type:      "application/epub+zip",
		Size:      637607,
		Payload: &storepb.AttachmentPayload{
			Payload: &storepb.AttachmentPayload_S3Object_{
				S3Object: &storepb.AttachmentPayload_S3Object{Key: "attachments/test.jpg", StorageId: "s3-primary"},
			},
			MotionMedia: &storepb.MotionMedia{
				Family:  storepb.MotionMediaFamily_APPLE_LIVE_PHOTO,
				Role:    storepb.MotionMediaRole_STILL,
				GroupId: "live-photo-pair",
			},
			MediaMetadata: &storepb.MediaMetadata{
				Width:  proto.Int32(1200),
				Height: proto.Int32(800),
				Details: &storepb.MediaMetadata_Photo{Photo: &storepb.PhotoMetadata{
					CameraMake:            "Test Camera",
					SourceExifOrientation: proto.Int32(6),
				}},
			},
		},
	})
	require.NoError(t, err)

	correctFilename := "test.epub"
	incorrectFilename := "test.png"
	attachment, err := ts.GetAttachment(ctx, &store.FindAttachment{
		Filename: &correctFilename,
	})
	require.NoError(t, err)
	require.Equal(t, correctFilename, attachment.Filename)
	require.Equal(t, int32(1), attachment.ID)
	require.Equal(t, int32(1200), attachment.Payload.GetMediaMetadata().GetWidth())
	require.Equal(t, "Test Camera", attachment.Payload.GetMediaMetadata().GetPhoto().GetCameraMake())
	require.Equal(t, int32(6), attachment.Payload.GetMediaMetadata().GetPhoto().GetSourceExifOrientation())
	require.Equal(t, "attachments/test.jpg", attachment.Payload.GetS3Object().GetKey())
	require.Equal(t, "s3-primary", attachment.Payload.GetS3Object().GetStorageId())
	require.Equal(t, "live-photo-pair", attachment.Payload.GetMotionMedia().GetGroupId())

	notFoundAttachment, err := ts.GetAttachment(ctx, &store.FindAttachment{
		Filename: &incorrectFilename,
	})
	require.NoError(t, err)
	require.Nil(t, notFoundAttachment)

	var correctCreatorID int32 = 101
	var incorrectCreatorID int32 = 102
	_, err = ts.GetAttachment(ctx, &store.FindAttachment{
		CreatorID: &correctCreatorID,
	})
	require.NoError(t, err)

	notFoundAttachment, err = ts.GetAttachment(ctx, &store.FindAttachment{
		CreatorID: &incorrectCreatorID,
	})
	require.NoError(t, err)
	require.Nil(t, notFoundAttachment)

	err = ts.DeleteAttachment(ctx, &store.DeleteAttachment{
		ID: 1,
	})
	require.NoError(t, err)
	err = ts.DeleteAttachment(ctx, &store.DeleteAttachment{
		ID: 2,
	})
	require.ErrorContains(t, err, "attachment not found")
	ts.Close()
}

func TestNoteMutationRollsBackOnBindingConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        shortuuid.New(),
		CreatorID:  user.ID,
		Content:    "original",
		Visibility: store.Private,
	})
	require.NoError(t, err)
	otherNote, err := ts.CreateNote(ctx, &store.Note{
		UID:        shortuuid.New(),
		CreatorID:  user.ID,
		Content:    "other",
		Visibility: store.Private,
	})
	require.NoError(t, err)
	available, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: user.ID,
		Filename:  "available.png",
		Type:      "image/png",
	})
	require.NoError(t, err)
	conflicting, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: user.ID,
		Filename:  "conflicting.png",
		Type:      "image/png",
		NoteID:    &otherNote.ID,
	})
	require.NoError(t, err)

	updatedContent := "updated"
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteID:              note.ID,
		NoteCreatorID:       user.ID,
		ExpectedNoteContent: note.Content,
		NoteUpdate:          &store.UpdateNote{ID: note.ID, Content: &updatedContent},
		Bindings: []*store.NoteAttachmentBinding{
			{ID: available.ID, UID: available.UID, UpdatedTs: time.Now().Unix()},
			{ID: conflicting.ID, UID: conflicting.UID, UpdatedTs: time.Now().Unix() + 1},
		},
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)

	storedNote, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, "original", storedNote.Content)
	storedAvailable, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &available.ID})
	require.NoError(t, err)
	require.Nil(t, storedAvailable.NoteID)
	storedConflicting, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &conflicting.ID})
	require.NoError(t, err)
	require.NotNil(t, storedConflicting.NoteID)
	require.Equal(t, otherNote.ID, *storedConflicting.NoteID)
}

func TestNoteCreationMutationRollsBackDependentWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	topLevelUID := shortuuid.New()
	topLevel := &store.Note{UID: topLevelUID, CreatorID: user.ID, Content: "top level", Visibility: store.Private}
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteCreate:            topLevel,
		NoteCreatorID:         user.ID,
		ExpectedNoteContent:   topLevel.Content,
		RequiredAttachmentIDs: []int32{2147483000},
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)
	storedTopLevel, err := ts.GetNote(ctx, &store.FindNote{UID: &topLevelUID})
	require.NoError(t, err)
	require.Nil(t, storedTopLevel, "dependent write failure must roll back the top-level note row")

	root, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "root", Visibility: store.Public})
	require.NoError(t, err)
	commentUID := shortuuid.New()
	comment := &store.Note{UID: commentUID, CreatorID: user.ID, Content: "comment", Visibility: store.Public}
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteCreate:            comment,
		CommentContextNoteID:  &root.ID,
		NoteCreatorID:         user.ID,
		ExpectedNoteContent:   comment.Content,
		RequiredAttachmentIDs: []int32{2147483000},
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)
	storedComment, err := ts.GetNote(ctx, &store.FindNote{UID: &commentUID})
	require.NoError(t, err)
	require.Nil(t, storedComment, "dependent write failure must roll back the comment row")
}

func TestNoteCreationMutationCommitsNoteBindingsAndRelationsTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	target, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "target", Visibility: store.Private})
	require.NoError(t, err)
	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: user.ID, Filename: "created.png", Type: "image/png",
	})
	require.NoError(t, err)

	created := &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "created", Visibility: store.Private}
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteCreate:                created,
		NoteCreatorID:             user.ID,
		ExpectedNoteContent:       created.Content,
		Bindings:                  []*store.NoteAttachmentBinding{{ID: attachment.ID, UID: attachment.UID, UpdatedTs: time.Now().Unix()}},
		RequiredAttachmentIDs:     []int32{attachment.ID},
		ReplaceReferenceRelations: true,
		ReferenceRelations: []*store.NoteRelation{{
			RelatedNoteID: target.ID,
			Type:          store.NoteRelationReference,
		}},
	})
	require.NoError(t, err)
	require.Positive(t, created.ID)

	storedAttachment, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.NotNil(t, storedAttachment.NoteID)
	require.Equal(t, created.ID, *storedAttachment.NoteID)
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteIDList: []int32{created.ID}})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, created.ID, relations[0].NoteID)
	require.Equal(t, target.ID, relations[0].RelatedNoteID)
}

func TestNoteMutationUpdatesNoteAndBindingsTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "old", Visibility: store.Private})
	require.NoError(t, err)
	removed, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: user.ID, Filename: "removed.png", Type: "image/png", NoteID: &note.ID,
	})
	require.NoError(t, err)
	added, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: user.ID, Filename: "added.png", Type: "image/png",
	})
	require.NoError(t, err)

	updatedContent := "new"
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteID:              note.ID,
		NoteCreatorID:       user.ID,
		ExpectedNoteContent: note.Content,
		NoteUpdate:          &store.UpdateNote{ID: note.ID, Content: &updatedContent},
		Bindings: []*store.NoteAttachmentBinding{
			{ID: added.ID, UID: added.UID, UpdatedTs: time.Now().Unix()},
		},
		RemovedAttachmentIDs:  []int32{removed.ID},
		RequiredAttachmentIDs: []int32{added.ID},
	})
	require.NoError(t, err)

	storedNote, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, "new", storedNote.Content)
	storedAdded, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &added.ID})
	require.NoError(t, err)
	require.NotNil(t, storedAdded.NoteID)
	require.Equal(t, note.ID, *storedAdded.NoteID)
	storedRemoved, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &removed.ID})
	require.NoError(t, err)
	require.Nil(t, storedRemoved)
}

func TestNoteMutationRollsBackWhenRequiredAttachmentIsMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "old", Visibility: store.Private})
	require.NoError(t, err)
	removed, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: user.ID, Filename: "kept.png", Type: "image/png", NoteID: &note.ID,
	})
	require.NoError(t, err)
	added, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: user.ID, Filename: "rolled-back.png", Type: "image/png",
	})
	require.NoError(t, err)

	updatedContent := "new"
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteID:              note.ID,
		NoteCreatorID:       user.ID,
		ExpectedNoteContent: note.Content,
		NoteUpdate:          &store.UpdateNote{ID: note.ID, Content: &updatedContent},
		Bindings: []*store.NoteAttachmentBinding{
			{ID: added.ID, UID: added.UID, UpdatedTs: time.Now().Unix()},
		},
		RemovedAttachmentIDs:  []int32{removed.ID},
		RequiredAttachmentIDs: []int32{added.ID + removed.ID + 1000},
	})
	require.ErrorIs(t, err, store.ErrNoteMutationConflict)

	storedNote, err := ts.GetNote(ctx, &store.FindNote{ID: &note.ID})
	require.NoError(t, err)
	require.Equal(t, "old", storedNote.Content)
	storedAdded, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &added.ID})
	require.NoError(t, err)
	require.Nil(t, storedAdded.NoteID)
	storedRemoved, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &removed.ID})
	require.NoError(t, err)
	require.NotNil(t, storedRemoved.NoteID)
	require.Equal(t, note.ID, *storedRemoved.NoteID)
}

func TestNoteMutationRejectsCommentInReferenceReplacement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	source, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "original", Visibility: store.Private})
	require.NoError(t, err)
	originalTarget, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "original target", Visibility: store.Private})
	require.NoError(t, err)
	replacementTarget, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "replacement target", Visibility: store.Private})
	require.NoError(t, err)
	_, err = ts.UpsertNoteRelation(ctx, &store.NoteRelation{
		NoteID: source.ID, RelatedNoteID: originalTarget.ID, Type: store.NoteRelationReference,
	})
	require.NoError(t, err)

	updatedContent := "must roll back"
	err = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
		NoteID:                    source.ID,
		NoteCreatorID:             user.ID,
		ExpectedNoteContent:       source.Content,
		NoteUpdate:                &store.UpdateNote{ID: source.ID, Content: &updatedContent},
		ReplaceReferenceRelations: true,
		ReferenceRelations: []*store.NoteRelation{
			{NoteID: source.ID, RelatedNoteID: replacementTarget.ID, Type: store.NoteRelationReference},
			{NoteID: source.ID, RelatedNoteID: originalTarget.ID, Type: store.NoteRelationComment},
		},
	})
	require.Error(t, err)

	stored, err := ts.GetNote(ctx, &store.FindNote{ID: &source.ID})
	require.NoError(t, err)
	require.Equal(t, "original", stored.Content)
	referenceType := store.NoteRelationReference
	relations, err := ts.ListNoteRelations(ctx, &store.FindNoteRelation{NoteID: &source.ID, Type: &referenceType})
	require.NoError(t, err)
	require.Len(t, relations, 1)
	require.Equal(t, originalTarget.ID, relations[0].RelatedNoteID)
}

func TestAttachmentStoreWithFilter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	_, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: 101,
		Filename:  "test.png",
		Blob:      []byte("test"),
		Type:      "image/png",
		Size:      1000,
	})
	require.NoError(t, err)

	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: 101,
		Filename:  "test.jpg",
		Blob:      []byte("test"),
		Type:      "image/jpeg",
		Size:      2000,
	})
	require.NoError(t, err)

	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: 101,
		Filename:  "test.pdf",
		Blob:      []byte("test"),
		Type:      "application/pdf",
		Size:      3000,
	})
	require.NoError(t, err)

	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &[]int32{101}[0],
		Filters:   []string{`mime_type == "image/png"`},
	})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, "image/png", attachments[0].Type)

	attachments, err = ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &[]int32{101}[0],
		Filters:   []string{`mime_type in ["image/png", "image/jpeg"]`},
	})
	require.NoError(t, err)
	require.Len(t, attachments, 2)

	attachments, err = ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &[]int32{101}[0],
		Filters:   []string{`filename.contains("test")`},
	})
	require.NoError(t, err)
	require.Len(t, attachments, 3)

	ts.Close()
}

func TestAttachmentUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       shortuuid.New(),
		CreatorID: 101,
		Filename:  "original.png",
		Blob:      []byte("test"),
		Type:      "image/png",
		Size:      1000,
	})
	require.NoError(t, err)

	// Update filename
	newFilename := "updated.png"
	err = ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID:       attachment.ID,
		Filename: &newFilename,
	})
	require.NoError(t, err)

	// Verify update
	found, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Equal(t, newFilename, found.Filename)

	ts.Close()
}

func TestAttachmentGetByUID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	uid := shortuuid.New()
	_, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       uid,
		CreatorID: 101,
		Filename:  "test.png",
		Blob:      []byte("test"),
		Type:      "image/png",
		Size:      1000,
	})
	require.NoError(t, err)

	// Get by UID
	found, err := ts.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, uid, found.UID)

	// Get non-existent UID
	nonExistentUID := "non-existent-uid"
	notFound, err := ts.GetAttachment(ctx, &store.FindAttachment{UID: &nonExistentUID})
	require.NoError(t, err)
	require.Nil(t, notFound)

	ts.Close()
}

func TestAttachmentListWithPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	// Create 5 attachments
	for i := range 5 {
		_, err := ts.CreateAttachment(ctx, &store.Attachment{
			UID:       shortuuid.New(),
			CreatorID: 101,
			Filename:  fmt.Sprintf("test%d.png", i),
			Blob:      []byte("test"),
			Type:      "image/png",
			Size:      int64(1000 + i),
		})
		require.NoError(t, err)
	}

	// Test limit
	limit := 3
	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &[]int32{101}[0],
		Limit:     &limit,
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(attachments))

	// Test offset
	offset := 2
	offsetAttachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		CreatorID: &[]int32{101}[0],
		Limit:     &limit,
		Offset:    &offset,
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(offsetAttachments))

	ts.Close()
}

func TestAttachmentInvalidUID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	// Create with invalid UID (contains spaces)
	_, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID:       "invalid uid with spaces",
		CreatorID: 101,
		Filename:  "test.png",
		Blob:      []byte("test"),
		Type:      "image/png",
		Size:      1000,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid uid")

	ts.Close()
}

// TestNoteMutationConcurrentUpdatesNoSQLiteBusy: concurrent distinct-note mutations all succeed (issue #6186).
func TestNoteMutationConcurrentUpdatesNoSQLiteBusy(t *testing.T) {
	if getDriverFromEnv() != "sqlite" {
		t.Skip("skipping sqlite lock regression test for non-sqlite driver")
	}
	t.Parallel()

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// One note per goroutine: writers don't share rows, so all must succeed.
	const workers = 10
	notes := make([]*store.Note, workers)
	for i := range notes {
		note, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "original", Visibility: store.Private})
		require.NoError(t, err)
		notes[i] = note
	}

	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	content := "updated"
	for i := range workers {
		wg.Go(func() {
			<-start
			errs[i] = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
				NoteID:              notes[i].ID,
				NoteCreatorID:       user.ID,
				ExpectedNoteContent: notes[i].Content,
				NoteUpdate:          &store.UpdateNote{ID: notes[i].ID, Content: &content},
			})
		})
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}

// TestNoteMutationConcurrentUpdatesSameNoteNoSQLiteBusy: same-note races only yield success or ErrNoteMutationConflict.
func TestNoteMutationConcurrentUpdatesSameNoteNoSQLiteBusy(t *testing.T) {
	if getDriverFromEnv() != "sqlite" {
		t.Skip("skipping sqlite lock regression test for non-sqlite driver")
	}
	t.Parallel()

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: user.ID, Content: "original", Visibility: store.Private})
	require.NoError(t, err)

	const workers = 10
	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	content := "updated"
	for i := range workers {
		wg.Go(func() {
			<-start
			errs[i] = ts.ApplyNoteMutation(ctx, &store.NoteMutation{
				NoteID:              note.ID,
				NoteCreatorID:       user.ID,
				ExpectedNoteContent: note.Content,
				NoteUpdate:          &store.UpdateNote{ID: note.ID, Content: &content},
			})
		})
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		if err == nil {
			continue
		}
		require.ErrorIs(t, err, store.ErrNoteMutationConflict)
	}
}

func TestAttachmentStorageUsage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	creator, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	size, err := ts.GetAttachmentStorageUsage(ctx, creator.ID)
	require.NoError(t, err)
	require.Zero(t, size)

	// One row per storage type, with a total exceeding a signed 32-bit integer.
	var expected int64
	var first *store.Attachment
	for i, storageType := range []storepb.AttachmentStorageType{
		storepb.AttachmentStorageType_ATTACHMENT_STORAGE_TYPE_UNSPECIFIED,
		storepb.AttachmentStorageType_LOCAL,
		storepb.AttachmentStorageType_S3,
	} {
		attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
			UID: shortuuid.New(), CreatorID: creator.ID, Filename: "file", Size: math.MaxInt32, StorageType: storageType,
		})
		require.NoError(t, err)
		expected += attachment.Size
		if i == 0 {
			first = attachment
		}
	}
	_, err = ts.CreateAttachment(ctx, &store.Attachment{UID: shortuuid.New(), CreatorID: 102, Filename: "other", Size: 123})
	require.NoError(t, err)
	// External links store no bytes, even when an import declared a size.
	_, err = ts.CreateAttachment(ctx, &store.Attachment{
		UID: shortuuid.New(), CreatorID: creator.ID, Filename: "link", Size: 456, StorageType: storepb.AttachmentStorageType_EXTERNAL, Reference: "https://example.com/file",
	})
	require.NoError(t, err)
	size, err = ts.GetAttachmentStorageUsage(ctx, creator.ID)
	require.NoError(t, err)
	require.Equal(t, expected, size, "external links do not count")

	note, err := ts.CreateNote(ctx, &store.Note{UID: shortuuid.New(), CreatorID: creator.ID, Content: "note", Visibility: store.Private})
	require.NoError(t, err)
	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: first.ID, NoteID: &note.ID}))
	size, err = ts.GetAttachmentStorageUsage(ctx, creator.ID)
	require.NoError(t, err)
	require.Equal(t, expected, size, "binding does not change storage usage")

	require.NoError(t, ts.DeleteAttachment(ctx, &store.DeleteAttachment{ID: first.ID}))
	size, err = ts.GetAttachmentStorageUsage(ctx, creator.ID)
	require.NoError(t, err)
	require.Equal(t, expected-first.Size, size)
}

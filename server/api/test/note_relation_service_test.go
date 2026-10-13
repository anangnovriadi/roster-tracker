package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
)

func TestSetNoteRelations(t *testing.T) {
	ctx := context.Background()

	t.Run("SetNoteRelations success by note owner", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Create note1
		note1, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{
				Content:    "Test note 1",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note1)

		// Create note2
		note2, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{
				Content:    "Test note 2",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note2)

		// Set note relations - should succeed
		_, err = ts.Service.SetNoteRelations(userCtx, &apipb.SetNoteRelationsRequest{
			Name: note1.Name,
			Relations: []*apipb.NoteRelation{
				{
					RelatedNote: &apipb.NoteRelation_NoteRef{
						Name: note2.Name,
					},
					Type: apipb.NoteRelation_REFERENCE,
				},
			},
		})
		require.NoError(t, err)
	})

	t.Run("SetNoteRelations host user has no ownership bypass", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create regular user
		regularUser, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		regularUserCtx := ts.CreateUserContext(ctx, regularUser.ID)

		// Create host user
		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		hostCtx := ts.CreateUserContext(ctx, hostUser.ID)

		// Create note by regular user
		note, err := ts.Service.CreateNote(regularUserCtx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{
				Content:    "Test note",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Application ADMIN is the superuser and manages any note's relations.
		_, err = ts.Service.SetNoteRelations(hostCtx, &apipb.SetNoteRelationsRequest{
			Name:      note.Name,
			Relations: []*apipb.NoteRelation{},
		})
		require.NoError(t, err)
	})

	t.Run("SetNoteRelations permission denied for non-owner", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user1
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user1Ctx := ts.CreateUserContext(ctx, user1.ID)

		// Create user2
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)
		user2Ctx := ts.CreateUserContext(ctx, user2.ID)

		// Create note by user1
		note, err := ts.Service.CreateNote(user1Ctx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{
				Content:    "Test note",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// User2 tries to modify relations - should fail
		_, err = ts.Service.SetNoteRelations(user2Ctx, &apipb.SetNoteRelationsRequest{
			Name:      note.Name,
			Relations: []*apipb.NoteRelation{},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("SetNoteRelations unauthenticated", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Create note
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{
				Content:    "Test note",
				Visibility: apipb.Visibility_PRIVATE,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Unauthenticated user tries to modify relations - should fail
		_, err = ts.Service.SetNoteRelations(ctx, &apipb.SetNoteRelationsRequest{
			Name:      note.Name,
			Relations: []*apipb.NoteRelation{},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("SetNoteRelations note not found", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Try to set relations on non-existent note - should fail
		_, err = ts.Service.SetNoteRelations(userCtx, &apipb.SetNoteRelationsRequest{
			Name:      "notes/nonexistent-uid-12345",
			Relations: []*apipb.NoteRelation{},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestUpdateNoteValidatesAllRelationsBeforeMutation(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	user, err := ts.CreateRegularUser(ctx, "atomic-relation-update")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	target, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: "target"}})
	require.NoError(t, err)
	replacement, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{Content: "replacement"}})
	require.NoError(t, err)
	source, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "original",
		Relations: []*apipb.NoteRelation{{
			RelatedNote: &apipb.NoteRelation_NoteRef{Name: target.Name},
			Type:        apipb.NoteRelation_REFERENCE,
		}},
	}})
	require.NoError(t, err)

	_, err = ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
		Note: &apipb.Note{
			Name:    source.Name,
			Content: "must roll back",
			Relations: []*apipb.NoteRelation{
				{RelatedNote: &apipb.NoteRelation_NoteRef{Name: replacement.Name}, Type: apipb.NoteRelation_REFERENCE},
				{RelatedNote: &apipb.NoteRelation_NoteRef{Name: "invalid-name"}, Type: apipb.NoteRelation_REFERENCE},
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content", "relations"}},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	stored, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: source.Name})
	require.NoError(t, err)
	require.Equal(t, "original", stored.Content)
	require.Len(t, stored.Relations, 1)
	require.Equal(t, target.Name, stored.Relations[0].RelatedNote.Name)
}

func TestSetNoteRelationsRejectsNonReferenceTypes(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "relation-type-owner")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	source, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "source", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)
	target, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "target", Visibility: apipb.Visibility_PRIVATE},
	})
	require.NoError(t, err)

	for _, test := range []struct {
		name         string
		relationType apipb.NoteRelation_Type
	}{
		{name: "unspecified", relationType: apipb.NoteRelation_TYPE_UNSPECIFIED},
		{name: "comment", relationType: apipb.NoteRelation_COMMENT},
		{name: "unknown", relationType: apipb.NoteRelation_Type(999)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ts.Service.SetNoteRelations(userCtx, &apipb.SetNoteRelationsRequest{
				Name: source.Name,
				Relations: []*apipb.NoteRelation{{
					RelatedNote: &apipb.NoteRelation_NoteRef{Name: target.Name},
					Type:        test.relationType,
				}},
			})
			require.Equal(t, codes.InvalidArgument, status.Code(err))

			stored, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: source.Name})
			require.NoError(t, err)
			require.Empty(t, stored.Relations)
		})
	}
}

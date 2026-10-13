package test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
)

func TestListNoteViews(t *testing.T) {
	ctx := context.Background()

	t.Run("ListNoteViews success", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// List noteViews (should be empty initially)
		req := &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
		}

		resp, err := ts.Service.ListNoteViews(userCtx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Empty(t, resp.NoteViews)
	})

	t.Run("ListNoteViews permission denied for different user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create two users
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)

		// Set user1 context but try to list user2's noteViews
		userCtx := ts.CreateUserContext(ctx, user1.ID)

		req := &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user2.Username),
		}

		_, err = ts.Service.ListNoteViews(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("ListNoteViews invalid parent format", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.ListNoteViewsRequest{
			Parent: "invalid-parent-format",
		}

		_, err = ts.Service.ListNoteViews(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid user name")
	})

	t.Run("ListNoteViews unauthenticated", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		req := &apipb.ListNoteViewsRequest{
			Parent: "users/testuser",
		}

		_, err = ts.Service.ListNoteViews(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "user not authenticated")
	})

	t.Run("ListNoteViews returns not found for numeric parent", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		userCtx := ts.CreateUserContext(ctx, user.ID)

		_, err = ts.Service.ListNoteViews(userCtx, &apipb.ListNoteViewsRequest{
			Parent: "users/1",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "user not found")
	})
}

func TestGetNoteView(t *testing.T) {
	ctx := context.Background()

	t.Run("GetNoteView success", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// First create a noteView
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Test NoteView",
				Filter: "tag in [\"test\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(userCtx, createReq)
		require.NoError(t, err)

		// Now get the noteView
		getReq := &apipb.GetNoteViewRequest{
			Name: created.Name,
		}

		resp, err := ts.Service.GetNoteView(userCtx, getReq)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, created.Name, resp.Name)
		require.Equal(t, "Test NoteView", resp.Title)
		require.Equal(t, "tag in [\"test\"]", resp.Filter)
	})

	t.Run("GetNoteView permission denied for different user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create two users
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)

		// Create noteView as user1
		user1Ctx := ts.CreateUserContext(ctx, user1.ID)
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user1.Username),
			NoteView: &apipb.NoteView{
				Title:  "User1 NoteView",
				Filter: "tag in [\"user1\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(user1Ctx, createReq)
		require.NoError(t, err)

		// Try to get noteView as user2
		user2Ctx := ts.CreateUserContext(ctx, user2.ID)
		getReq := &apipb.GetNoteViewRequest{
			Name: created.Name,
		}

		_, err = ts.Service.GetNoteView(user2Ctx, getReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("GetNoteView invalid name format", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.GetNoteViewRequest{
			Name: "invalid-note-view-name",
		}

		_, err = ts.Service.GetNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid note view name")
	})

	t.Run("GetNoteView not found", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.GetNoteViewRequest{
			Name: fmt.Sprintf("users/%s", user.Username) + "/views/nonexistent",
		}

		_, err = ts.Service.GetNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestCreateNoteView(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateNoteView success", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "My NoteView",
				Filter: "tag in [\"important\"]",
			},
		}

		resp, err := ts.Service.CreateNoteView(userCtx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "My NoteView", resp.Title)
		require.Equal(t, "tag in [\"important\"]", resp.Filter)
		require.Contains(t, resp.Name, fmt.Sprintf("users/%s/views/", user.Username))

		// Verify the noteView was created by listing
		listReq := &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
		}

		listResp, err := ts.Service.ListNoteViews(userCtx, listReq)
		require.NoError(t, err)
		require.Len(t, listResp.NoteViews, 1)
		require.Equal(t, "My NoteView", listResp.NoteViews[0].Title)
	})

	t.Run("CreateNoteView permission denied for different user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create two users
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)

		// Set user1 context but try to create noteView for user2
		userCtx := ts.CreateUserContext(ctx, user1.ID)

		req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user2.Username),
			NoteView: &apipb.NoteView{
				Title:  "Forbidden NoteView",
				Filter: "tag in [\"forbidden\"]",
			},
		}

		_, err = ts.Service.CreateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("CreateNoteView invalid parent format", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.CreateNoteViewRequest{
			Parent: "invalid-parent",
			NoteView: &apipb.NoteView{
				Title:  "Test NoteView",
				Filter: "tag in [\"test\"]",
			},
		}

		_, err = ts.Service.CreateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid user name")
	})

	t.Run("CreateNoteView invalid filter", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Invalid Filter NoteView",
				Filter: "invalid||filter))syntax",
			},
		}

		_, err = ts.Service.CreateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid filter")
	})

	t.Run("CreateNoteView missing title", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Filter: "tag in [\"test\"]",
			},
		}

		_, err = ts.Service.CreateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "title is required")
	})
}

func TestUpdateNoteView(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateNoteView success", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Create a noteView first
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Original Title",
				Filter: "tag in [\"original\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(userCtx, createReq)
		require.NoError(t, err)

		// Update the noteView
		updateReq := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:   created.Name,
				Title:  "Updated Title",
				Filter: "tag in [\"updated\"]",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title", "filter"},
			},
		}

		updated, err := ts.Service.UpdateNoteView(userCtx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, updated)
		require.Equal(t, "Updated Title", updated.Title)
		require.Equal(t, "tag in [\"updated\"]", updated.Filter)
		require.Equal(t, created.Name, updated.Name)
	})

	t.Run("UpdateNoteView permission denied for different user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create two users
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)

		// Create noteView as user1
		user1Ctx := ts.CreateUserContext(ctx, user1.ID)
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user1.Username),
			NoteView: &apipb.NoteView{
				Title:  "User1 NoteView",
				Filter: "tag in [\"user1\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(user1Ctx, createReq)
		require.NoError(t, err)

		// Try to update noteView as user2
		user2Ctx := ts.CreateUserContext(ctx, user2.ID)
		updateReq := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:   created.Name,
				Title:  "Hacked Title",
				Filter: "tag in [\"hacked\"]",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title", "filter"},
			},
		}

		_, err = ts.Service.UpdateNoteView(user2Ctx, updateReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("UpdateNoteView missing update mask", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user and context for authentication
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:  fmt.Sprintf("users/%s/views/test", user.Username),
				Title: "Updated Title",
			},
		}

		_, err = ts.Service.UpdateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "update mask is required")
	})

	t.Run("UpdateNoteView rejects unsupported update mask paths", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		created, err := ts.Service.CreateNoteView(userCtx, &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Original Title",
				Filter: `tag in ["original"]`,
			},
		})
		require.NoError(t, err)

		_, err = ts.Service.UpdateNoteView(userCtx, &apipb.UpdateNoteViewRequest{
			NoteView:   created,
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported update mask path: name")
	})

	t.Run("UpdateNoteView invalid name format", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		req := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:  "invalid-note-view-name",
				Title: "Updated Title",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title"},
			},
		}

		_, err := ts.Service.UpdateNoteView(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid note view name")
	})

	t.Run("UpdateNoteView invalid filter", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Create a noteView first
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Test NoteView",
				Filter: "tag in [\"test\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(userCtx, createReq)
		require.NoError(t, err)

		// Try to update with invalid filter
		updateReq := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:   created.Name,
				Filter: "invalid||filter))syntax",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"filter"},
			},
		}

		_, err = ts.Service.UpdateNoteView(userCtx, updateReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid filter")
	})
}

func TestDeleteNoteView(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteNoteView success", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Create a noteView first
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "NoteView to Delete",
				Filter: "tag in [\"delete\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(userCtx, createReq)
		require.NoError(t, err)

		// Delete the noteView
		deleteReq := &apipb.DeleteNoteViewRequest{
			Name: created.Name,
		}

		_, err = ts.Service.DeleteNoteView(userCtx, deleteReq)
		require.NoError(t, err)

		// Verify deletion by listing noteViews
		listReq := &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
		}

		listResp, err := ts.Service.ListNoteViews(userCtx, listReq)
		require.NoError(t, err)
		require.Empty(t, listResp.NoteViews)

		// Also verify by trying to get the deleted noteView
		getReq := &apipb.GetNoteViewRequest{
			Name: created.Name,
		}

		_, err = ts.Service.GetNoteView(userCtx, getReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("DeleteNoteView permission denied for different user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create two users
		user1, err := ts.CreateRegularUser(ctx, "user1")
		require.NoError(t, err)
		user2, err := ts.CreateRegularUser(ctx, "user2")
		require.NoError(t, err)

		// Create noteView as user1
		user1Ctx := ts.CreateUserContext(ctx, user1.ID)
		createReq := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user1.Username),
			NoteView: &apipb.NoteView{
				Title:  "User1 NoteView",
				Filter: "tag in [\"user1\"]",
			},
		}

		created, err := ts.Service.CreateNoteView(user1Ctx, createReq)
		require.NoError(t, err)

		// Try to delete noteView as user2
		user2Ctx := ts.CreateUserContext(ctx, user2.ID)
		deleteReq := &apipb.DeleteNoteViewRequest{
			Name: created.Name,
		}

		_, err = ts.Service.DeleteNoteView(user2Ctx, deleteReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("DeleteNoteView invalid name format", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		req := &apipb.DeleteNoteViewRequest{
			Name: "invalid-note-view-name",
		}

		_, err := ts.Service.DeleteNoteView(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid note view name")
	})

	t.Run("DeleteNoteView not found", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.DeleteNoteViewRequest{
			Name: fmt.Sprintf("users/%s", user.Username) + "/views/nonexistent",
		}

		_, err = ts.Service.DeleteNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestNoteViewFiltering(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateNoteView with valid filters", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Test various valid filter formats
		validFilters := []string{
			"tag in [\"work\"]",
			"content.contains(\"meeting\")",
			"tag in [\"work\"] && content.contains(\"meeting\")",
			"tag in [\"work\"] || tag in [\"personal\"]",
			"creator_id == 1",
			"visibility == \"PUBLIC\"",
			"has_task_list == true",
			"has_task_list == false",
		}

		for i, filter := range validFilters {
			req := &apipb.CreateNoteViewRequest{
				Parent: fmt.Sprintf("users/%s", user.Username),
				NoteView: &apipb.NoteView{
					Title:  fmt.Sprintf("Valid Filter %d", i),
					Filter: filter,
				},
			}

			_, err = ts.Service.CreateNoteView(userCtx, req)
			require.NoError(t, err, "Filter should be valid: %s", filter)
		}
	})

	t.Run("CreateNoteView with invalid filters", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Test various invalid filter formats
		invalidFilters := []string{
			"tag in ",                                   // incomplete expression
			"invalid_field @in [\"value\"]",             // unknown field
			"tag in [\"work\"] &&",                      // incomplete expression
			"tag in [\"work\"] || || tag in [\"test\"]", // double operator
			"((tag in [\"work\"]",                       // unmatched parentheses
			"tag in [\"work\"] && )",                    // mismatched parentheses
			"tag == \"work\"",                           // wrong operator (== not supported for tags)
			"tag in work",                               // missing brackets
		}

		for _, filter := range invalidFilters {
			req := &apipb.CreateNoteViewRequest{
				Parent: fmt.Sprintf("users/%s", user.Username),
				NoteView: &apipb.NoteView{
					Title:  "Invalid Filter Test",
					Filter: filter,
				},
			}

			_, err = ts.Service.CreateNoteView(userCtx, req)
			require.Error(t, err, "Filter should be invalid: %s", filter)
			require.Contains(t, err.Error(), "invalid filter", "Error should mention invalid filter for: %s", filter)
		}
	})
}

func TestNoteViewCRUDComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("Complete CRUD lifecycle", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Set user context
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// 1. Create multiple noteViews
		noteView1Req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Work Notes",
				Filter: "tag in [\"work\"]",
			},
		}

		noteView2Req := &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Personal Notes",
				Filter: "tag in [\"personal\"]",
			},
		}

		created1, err := ts.Service.CreateNoteView(userCtx, noteView1Req)
		require.NoError(t, err)
		require.Equal(t, "Work Notes", created1.Title)

		created2, err := ts.Service.CreateNoteView(userCtx, noteView2Req)
		require.NoError(t, err)
		require.Equal(t, "Personal Notes", created2.Title)

		// 2. List noteViews and verify both exist
		listReq := &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
		}

		listResp, err := ts.Service.ListNoteViews(userCtx, listReq)
		require.NoError(t, err)
		require.Len(t, listResp.NoteViews, 2)

		// 3. Get individual noteViews
		getReq1 := &apipb.GetNoteViewRequest{Name: created1.Name}
		getResp1, err := ts.Service.GetNoteView(userCtx, getReq1)
		require.NoError(t, err)
		require.Equal(t, created1.Name, getResp1.Name)
		require.Equal(t, "Work Notes", getResp1.Title)

		getReq2 := &apipb.GetNoteViewRequest{Name: created2.Name}
		getResp2, err := ts.Service.GetNoteView(userCtx, getReq2)
		require.NoError(t, err)
		require.Equal(t, created2.Name, getResp2.Name)
		require.Equal(t, "Personal Notes", getResp2.Title)

		// 4. Update one noteView
		updateReq := &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:   created1.Name,
				Title:  "Work & Meeting Notes",
				Filter: "tag in [\"work\"] || tag in [\"meeting\"]",
			},
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{"title", "filter"},
			},
		}

		updated, err := ts.Service.UpdateNoteView(userCtx, updateReq)
		require.NoError(t, err)
		require.Equal(t, "Work & Meeting Notes", updated.Title)
		require.Equal(t, "tag in [\"work\"] || tag in [\"meeting\"]", updated.Filter)

		// 5. Verify update by getting it again
		getUpdatedReq := &apipb.GetNoteViewRequest{Name: created1.Name}
		getUpdatedResp, err := ts.Service.GetNoteView(userCtx, getUpdatedReq)
		require.NoError(t, err)
		require.Equal(t, "Work & Meeting Notes", getUpdatedResp.Title)
		require.Equal(t, "tag in [\"work\"] || tag in [\"meeting\"]", getUpdatedResp.Filter)

		// 6. Delete one noteView
		deleteReq := &apipb.DeleteNoteViewRequest{
			Name: created2.Name,
		}

		_, err = ts.Service.DeleteNoteView(userCtx, deleteReq)
		require.NoError(t, err)

		// 7. Verify deletion by listing (should only have 1 left)
		finalListResp, err := ts.Service.ListNoteViews(userCtx, listReq)
		require.NoError(t, err)
		require.Len(t, finalListResp.NoteViews, 1)
		require.Equal(t, "Work & Meeting Notes", finalListResp.NoteViews[0].Title)

		// 8. Verify deleted noteView can't be accessed
		getDeletedReq := &apipb.GetNoteViewRequest{Name: created2.Name}
		_, err = ts.Service.GetNoteView(userCtx, getDeletedReq)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})
}

func TestNoteViewRegressions(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateNoteView rejects a missing note view instead of panicking", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		req := &apipb.UpdateNoteViewRequest{
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
		}

		_, err = ts.Service.UpdateNoteView(userCtx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid note view name")
	})

	t.Run("UpdateNoteView leaves the stored view untouched when validation fails", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		created, err := ts.Service.CreateNoteView(userCtx, &apipb.CreateNoteViewRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
			NoteView: &apipb.NoteView{
				Title:  "Original Title",
				Filter: `tag in ["work"]`,
			},
		})
		require.NoError(t, err)

		// The title path is applied before the filter path is validated. A rejected
		// filter must not leave the new title behind.
		_, err = ts.Service.UpdateNoteView(userCtx, &apipb.UpdateNoteViewRequest{
			NoteView: &apipb.NoteView{
				Name:   created.Name,
				Title:  "Phantom Title",
				Filter: "this is not (((a valid filter",
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "filter"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid filter")

		got, err := ts.Service.GetNoteView(userCtx, &apipb.GetNoteViewRequest{Name: created.Name})
		require.NoError(t, err)
		require.Equal(t, "Original Title", got.Title)
		require.Equal(t, `tag in ["work"]`, got.Filter)
	})

	t.Run("CreateNoteView keeps every view under concurrent creates", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		const concurrency = 8
		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		for i := range concurrency {
			wg.Go(func() {
				_, errs[i] = ts.Service.CreateNoteView(userCtx, &apipb.CreateNoteViewRequest{
					Parent: fmt.Sprintf("users/%s", user.Username),
					NoteView: &apipb.NoteView{
						Title:  fmt.Sprintf("View %d", i),
						Filter: `tag in ["work"]`,
					},
				})
			})
		}
		wg.Wait()
		for _, err := range errs {
			require.NoError(t, err)
		}

		resp, err := ts.Service.ListNoteViews(userCtx, &apipb.ListNoteViewsRequest{
			Parent: fmt.Sprintf("users/%s", user.Username),
		})
		require.NoError(t, err)
		require.Len(t, resp.NoteViews, concurrency)
	})
}

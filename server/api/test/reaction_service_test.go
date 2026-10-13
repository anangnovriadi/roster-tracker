package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteReactionResourceNames(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "reaction-resource-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		NoteId: "reaction-resource-note",
		Note: &apipb.Note{
			Content:    "reaction resource names",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	reaction, err := ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "👍",
		},
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(reaction.Name, note.Name+"/reactions/"))

	noteID := parseNoteIDFromNameForTest(t, ts, note.Name)
	storedReaction, err := ts.Store.GetReaction(ctx, &store.FindReaction{NoteID: &noteID, CreatorID: &user.ID})
	require.NoError(t, err)
	require.NotNil(t, storedReaction)
	require.Equal(t, noteID, storedReaction.NoteID)

	listed, err := ts.Service.ListNoteReactions(ctx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 1)
	require.Equal(t, reaction.Name, listed.Reactions[0].Name)

	fetchedNote, err := ts.Service.GetNote(ctx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
	require.Len(t, fetchedNote.Reactions, 1)
	require.Equal(t, reaction.Name, fetchedNote.Reactions[0].Name)
}

func TestUpsertNoteReactionRequiresReaction(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "reaction-required-user")
	require.NoError(t, err)
	_, err = ts.Service.UpsertNoteReaction(ts.CreateUserContext(ctx, user.ID), &apipb.UpsertNoteReactionRequest{
		Parent: "notes/missing",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestListNoteCommentsIncludesReactionResourceNames(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "comment-reaction-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	parent, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "parent", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)
	comment, err := ts.Service.CreateNoteComment(userCtx, &apipb.CreateNoteCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Note{Content: "comment", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	_, err = ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
		Parent: comment.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "🔥",
		},
	})
	require.NoError(t, err)

	comments, err := ts.Service.ListNoteComments(userCtx, &apipb.ListNoteCommentsRequest{Parent: parent.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1)
	require.Len(t, comments.Comments[0].Reactions, 1)
	require.True(t, strings.HasPrefix(comments.Comments[0].Reactions[0].Name, comment.Name+"/reactions/"))
}

func TestUpsertNoteReactionRevalidatesSpaceParticipation(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "reaction-space-owner")
	require.NoError(t, err)
	member, err := ts.CreateRegularUser(ctx, "reaction-space-member")
	require.NoError(t, err)
	applicationAdmin, err := ts.CreateHostUser(ctx, "reaction-space-application-admin")
	require.NoError(t, err)
	memberCtx := ts.CreateUserContext(ctx, member.ID)
	adminCtx := ts.CreateUserContext(ctx, applicationAdmin.ID)

	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "reaction-space", Title: "Reaction Space"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  member.ID,
		Role:    store.SpaceMemberRoleUser,
	}, owner.ID)
	require.NoError(t, err)
	root, err := ts.Store.CreateNote(ctx, &store.Note{
		UID:        "reaction-space-root",
		CreatorID:  owner.ID,
		Content:    "assigned public note",
		Visibility: store.Public,
		SpaceID:    &space.ID,
	})
	require.NoError(t, err)
	noteName := "notes/" + root.UID

	memberReaction, err := ts.Service.UpsertNoteReaction(memberCtx, &apipb.UpsertNoteReactionRequest{
		Parent:   noteName,
		Reaction: &apipb.Reaction{ReactionType: "👍"},
	})
	require.NoError(t, err)

	// Admins only read notes and cannot react.
	_, err = ts.Service.UpsertNoteReaction(adminCtx, &apipb.UpsertNoteReactionRequest{
		Parent:   noteName,
		Reaction: &apipb.Reaction{ReactionType: "🔥"},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: member.ID}, owner.ID))
	_, err = ts.Service.DeleteNoteReaction(memberCtx, &apipb.DeleteNoteReactionRequest{Name: memberReaction.Name})
	require.NoError(t, err, "the creator may withdraw a reaction after leaving")
	_, err = ts.Service.UpsertNoteReaction(memberCtx, &apipb.UpsertNoteReactionRequest{
		Parent:   noteName,
		Reaction: &apipb.Reaction{ReactionType: "🔥"},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  member.ID,
		Role:    store.SpaceMemberRoleUser,
	}, owner.ID)
	require.NoError(t, err)

	_, err = ts.Service.UpsertNoteReaction(memberCtx, &apipb.UpsertNoteReactionRequest{
		Parent:   noteName,
		Reaction: &apipb.Reaction{ReactionType: "🔥"},
	})
	require.NoError(t, err)

	reactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{NoteID: &root.ID})
	require.NoError(t, err)
	require.Len(t, reactions, 1)
}

func TestAssignedNoteReactionsFollowNoteReadAccess(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "assigned-reaction-owner")
	require.NoError(t, err)
	member, err := ts.CreateRegularUser(ctx, "assigned-reaction-member")
	require.NoError(t, err)
	outsider, err := ts.CreateRegularUser(ctx, "assigned-reaction-outsider")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	memberCtx := ts.CreateUserContext(ctx, member.ID)
	outsiderCtx := ts.CreateUserContext(ctx, outsider.ID)

	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "assigned-reaction-space", Title: "Assigned reactions"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID,
		UserID:  member.ID,
		Role:    store.SpaceMemberRoleAdmin,
	}, owner.ID)
	require.NoError(t, err)
	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content:    "assigned public note with reactions",
		Visibility: apipb.Visibility_PUBLIC,
		Space:      new("spaces/" + space.UID),
	}})
	require.NoError(t, err)
	_, err = ts.Service.UpsertNoteReaction(ownerCtx, &apipb.UpsertNoteReactionRequest{
		Parent:   note.Name,
		Reaction: &apipb.Reaction{ReactionType: "👍"},
	})
	require.NoError(t, err)

	listed, err := ts.Service.ListNoteReactions(memberCtx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 1)

	listed, err = ts.Service.ListNoteReactions(ctx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 1, "anonymous reaction reads follow the PUBLIC note audience")
	listed, err = ts.Service.ListNoteReactions(outsiderCtx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 1, "Space placement adds no reaction read gate")

	got, err := ts.Service.GetNote(outsiderCtx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err, "a non-member may still read an assigned PUBLIC note")
	require.Empty(t, got.GetSpace())
	require.Len(t, got.Reactions, 1, "embedded reactions follow the readable note")
	feed, err := ts.Service.ListNotes(outsiderCtx, &apipb.ListNotesRequest{PageSize: 20})
	require.NoError(t, err)
	var listedNote *apipb.Note
	for _, candidate := range feed.Notes {
		if candidate.Name == note.Name {
			listedNote = candidate
			break
		}
	}
	require.NotNil(t, listedNote)
	require.Len(t, listedNote.Reactions, 1)

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, owner.ID))
	listed, err = ts.Service.ListNoteReactions(ownerCtx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 1, "the removed author still reads a PUBLIC note and its reactions")
	got, err = ts.Service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: note.Name})
	require.NoError(t, err)
	require.Equal(t, "spaces/"+space.UID, got.GetSpace(), "the author may still see placement")
	require.Len(t, got.Reactions, 1)
}

func TestDeleteNoteReaction(t *testing.T) {
	ctx := context.Background()

	t.Run("DeleteNoteReaction success by reaction owner", func(t *testing.T) {
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
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Create reaction
		reaction, err := ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
			Parent: note.Name,
			Reaction: &apipb.Reaction{
				ReactionType: "👍",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, reaction)
		require.Equal(t, "users/user", reaction.Creator)

		// Delete reaction - should succeed
		_, err = ts.Service.DeleteNoteReaction(userCtx, &apipb.DeleteNoteReactionRequest{
			Name: reaction.Name,
		})
		require.NoError(t, err)
	})

	t.Run("DeleteNoteReaction fails closed when note was concurrently deleted", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "concurrent-delete-user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{
			Note: &apipb.Note{Content: "concurrent delete", Visibility: apipb.Visibility_PUBLIC},
		})
		require.NoError(t, err)
		reaction, err := ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
			Parent: note.Name,
			Reaction: &apipb.Reaction{
				ReactionType: "👍",
			},
		})
		require.NoError(t, err)

		noteID := parseNoteIDFromNameForTest(t, ts, note.Name)
		storedReaction, err := ts.Store.GetReaction(ctx, &store.FindReaction{NoteID: &noteID, CreatorID: &user.ID})
		require.NoError(t, err)
		require.NotNil(t, storedReaction)
		// Simulate the state visible between DeleteNoteReaction's reaction and
		// note reads after a concurrent note deletion commits.
		_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, "DELETE FROM note WHERE id = ?", noteID)
		require.NoError(t, err)

		_, err = ts.Service.DeleteNoteReaction(userCtx, &apipb.DeleteNoteReactionRequest{Name: reaction.Name})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		remainingReaction, err := ts.Store.GetReaction(ctx, &store.FindReaction{ID: &storedReaction.ID})
		require.NoError(t, err)
		require.NotNil(t, remainingReaction, "a missing direct note must not authorize a reaction mutation")
	})

	t.Run("DeleteNoteReaction host user has no ownership bypass", func(t *testing.T) {
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
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Create reaction by regular user
		reaction, err := ts.Service.UpsertNoteReaction(regularUserCtx, &apipb.UpsertNoteReactionRequest{
			Parent: note.Name,
			Reaction: &apipb.Reaction{
				ReactionType: "👍",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, reaction)

		// Admins only read notes and cannot remove a member's reaction.
		_, err = ts.Service.DeleteNoteReaction(hostCtx, &apipb.DeleteNoteReactionRequest{
			Name: reaction.Name,
		})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		noteID := parseNoteIDFromNameForTest(t, ts, note.Name)
		remainingReaction, err := ts.Store.GetReaction(ctx, &store.FindReaction{NoteID: &noteID, CreatorID: &regularUser.ID})
		require.NoError(t, err)
		require.NotNil(t, remainingReaction)
	})

	t.Run("DeleteNoteReaction permission denied for non-owner", func(t *testing.T) {
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
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Create reaction by user1
		reaction, err := ts.Service.UpsertNoteReaction(user1Ctx, &apipb.UpsertNoteReactionRequest{
			Parent: note.Name,
			Reaction: &apipb.Reaction{
				ReactionType: "👍",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, reaction)

		// User2 tries to delete reaction - should fail with permission denied
		_, err = ts.Service.DeleteNoteReaction(user2Ctx, &apipb.DeleteNoteReactionRequest{
			Name: reaction.Name,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("DeleteNoteReaction unauthenticated", func(t *testing.T) {
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
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, note)

		// Create reaction
		reaction, err := ts.Service.UpsertNoteReaction(userCtx, &apipb.UpsertNoteReactionRequest{
			Parent: note.Name,
			Reaction: &apipb.Reaction{
				ReactionType: "👍",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, reaction)

		// Unauthenticated user tries to delete reaction - should fail
		_, err = ts.Service.DeleteNoteReaction(ctx, &apipb.DeleteNoteReactionRequest{
			Name: reaction.Name,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("DeleteNoteReaction not found returns permission denied", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create user
		user, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)

		// Try to delete non-existent reaction - should fail with permission denied
		// (not "not found" to avoid information disclosure)
		// Use new nested resource format: notes/{note}/reactions/{reaction}
		_, err = ts.Service.DeleteNoteReaction(userCtx, &apipb.DeleteNoteReactionRequest{
			Name: "notes/nonexistent/reactions/99999",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
		require.NotContains(t, err.Error(), "not found")
	})
}

func TestListNoteReactionsSkipsMissingCreators(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "reaction-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	reactor, err := ts.CreateRegularUser(ctx, "reaction-orphan")
	require.NoError(t, err)
	reactorCtx := ts.CreateUserContext(ctx, reactor.ID)

	note, err := ts.Service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "reaction list note",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	_, err = ts.Service.UpsertNoteReaction(reactorCtx, &apipb.UpsertNoteReactionRequest{
		Parent: note.Name,
		Reaction: &apipb.Reaction{
			ReactionType: "🔥",
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: reactor.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListNoteReactions(ctx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Empty(t, resp.Reactions)
}

func TestUpsertNoteReactionAllowsOneReactionPerMember(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	author, err := ts.CreateRegularUser(ctx, "reaction-once-author")
	require.NoError(t, err)
	reader, err := ts.CreateRegularUser(ctx, "reaction-once-reader")
	require.NoError(t, err)
	authorCtx := ts.CreateUserContext(ctx, author.ID)
	readerCtx := ts.CreateUserContext(ctx, reader.ID)
	note, err := ts.Service.CreateNote(authorCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{Content: "one reaction each", Visibility: apipb.Visibility_PUBLIC},
	})
	require.NoError(t, err)

	react := func(ctx context.Context, emoji string) (*apipb.Reaction, error) {
		return ts.Service.UpsertNoteReaction(ctx, &apipb.UpsertNoteReactionRequest{
			Parent:   note.Name,
			Reaction: &apipb.Reaction{ReactionType: emoji},
		})
	}

	first, err := react(readerCtx, "👍")
	require.NoError(t, err)
	_, err = react(readerCtx, "😄")
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	_, err = react(readerCtx, "👍")
	require.Equal(t, codes.AlreadyExists, status.Code(err))

	// Another member still has their own reaction.
	_, err = react(authorCtx, "😄")
	require.NoError(t, err)

	// Removing the reaction frees the member to react again.
	_, err = ts.Service.DeleteNoteReaction(readerCtx, &apipb.DeleteNoteReactionRequest{Name: first.Name})
	require.NoError(t, err)
	_, err = react(readerCtx, "😄")
	require.NoError(t, err)

	listed, err := ts.Service.ListNoteReactions(ctx, &apipb.ListNoteReactionsRequest{Parent: note.Name})
	require.NoError(t, err)
	require.Len(t, listed.Reactions, 2)
}

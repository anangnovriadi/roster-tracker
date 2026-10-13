package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
	"github.com/anangnovriadi/roster-tracker/store/db"
)

func TestReactionStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-store")

	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "💗",
	})
	require.NoError(t, err)
	require.NotNil(t, reaction)
	require.NotEmpty(t, reaction.ID)

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.Len(t, reactions, 1)
	require.Equal(t, reaction, reactions[0])

	// Test GetReaction.
	gotReaction, err := ts.GetReaction(ctx, &store.FindReaction{
		ID: &reaction.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, gotReaction)
	require.Equal(t, reaction.ID, gotReaction.ID)
	require.Equal(t, reaction.CreatorID, gotReaction.CreatorID)
	require.Equal(t, reaction.NoteID, gotReaction.NoteID)
	require.Equal(t, reaction.ReactionType, gotReaction.ReactionType)

	// Test GetReaction with non-existent ID.
	nonExistentID := int32(99999)
	notFoundReaction, err := ts.GetReaction(ctx, &store.FindReaction{
		ID: &nonExistentID,
	})
	require.NoError(t, err)
	require.Nil(t, notFoundReaction)

	// An empty delete filter must not remove every reaction.
	require.NoError(t, ts.DeleteReaction(ctx, &store.DeleteReaction{}))
	reactions, err = ts.ListReactions(ctx, &store.FindReaction{NoteID: &note.ID})
	require.NoError(t, err)
	require.Len(t, reactions, 1)

	err = ts.DeleteReaction(ctx, &store.DeleteReaction{
		ID: &reaction.ID,
	})
	require.NoError(t, err)

	reactions, err = ts.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.Len(t, reactions, 0)

	ts.Close()
}

func TestReactionDeleteByNoteID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note1 := createReactionTestNote(ctx, t, ts, user.ID, "reaction-delete-note-1")
	note2 := createReactionTestNote(ctx, t, ts, user.ID, "reaction-delete-note-2")

	for _, reaction := range []*store.Reaction{
		{CreatorID: user.ID, NoteID: note1.ID, ReactionType: "👍"},
		{CreatorID: user.ID, NoteID: note1.ID, ReactionType: "❤️"},
		{CreatorID: user.ID, NoteID: note2.ID, ReactionType: "👍"},
	} {
		_, err := ts.UpsertReaction(ctx, reaction)
		require.NoError(t, err)
	}

	require.NoError(t, ts.DeleteReaction(ctx, &store.DeleteReaction{NoteID: &note1.ID}))
	reactions, err := ts.ListReactions(ctx, &store.FindReaction{})
	require.NoError(t, err)
	require.Len(t, reactions, 1)
	require.Equal(t, note2.ID, reactions[0].NoteID)

	ts.Close()
}

func TestReactionRejectsDeletedNote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-deleted-note")
	require.NoError(t, ts.DeleteNote(ctx, &store.DeleteNote{ID: note.ID}))

	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "👍",
	})
	require.ErrorIs(t, err, store.ErrReactionNoteNotFound)
	require.Nil(t, reaction)

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{NoteID: &note.ID})
	require.NoError(t, err)
	require.Empty(t, reactions)

	existingNote := createReactionTestNote(ctx, t, ts, user.ID, "reaction-existing-after-rejection")
	reaction, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       existingNote.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)
	require.NotNil(t, reaction)
	require.Equal(t, existingNote.ID, reaction.NoteID)

	ts.Close()
}

func TestReactionListByCreatorID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user1, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	user2, err := createTestingUserWithRole(ctx, ts, "user2", store.RoleUser)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user1.ID, "reaction-list-by-creator")

	// User 1 creates reaction
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user1.ID,
		NoteID:       note.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)

	// User 2 creates reaction
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user2.ID,
		NoteID:       note.ID,
		ReactionType: "❤️",
	})
	require.NoError(t, err)

	// List all reactions for the note.
	reactions, err := ts.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.Len(t, reactions, 2)

	// List by creator ID
	user1Reactions, err := ts.ListReactions(ctx, &store.FindReaction{
		CreatorID: &user1.ID,
	})
	require.NoError(t, err)
	require.Len(t, user1Reactions, 1)
	require.Equal(t, "👍", user1Reactions[0].ReactionType)

	ts.Close()
}

func TestReactionMultipleNoteIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note1 := createReactionTestNote(ctx, t, ts, user.ID, "reaction-note-1")
	note2 := createReactionTestNote(ctx, t, ts, user.ID, "reaction-note-2")
	note3 := createReactionTestNote(ctx, t, ts, user.ID, "reaction-note-3")

	// Create reactions for different notes.
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note1.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)

	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note2.ID,
		ReactionType: "❤️",
	})
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note3.ID,
		ReactionType: "🔥",
	})
	require.NoError(t, err)

	// List by note ID list, excluding the third note.
	reactions, err := ts.ListReactions(ctx, &store.FindReaction{
		NoteIDList: []int32{note1.ID, note2.ID},
	})
	require.NoError(t, err)
	require.Len(t, reactions, 2)
	require.ElementsMatch(t, []int32{note1.ID, note2.ID}, []int32{reactions[0].NoteID, reactions[1].NoteID})

	reaction, err := ts.GetReaction(ctx, &store.FindReaction{NoteIDList: []int32{note2.ID}})
	require.NoError(t, err)
	require.NotNil(t, reaction)
	require.Equal(t, note2.ID, reaction.NoteID)

	ts.Close()
}

func TestReactionUpsertDifferentTypes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-different-types")

	// Create first reaction
	reaction1, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)

	// Create second reaction with different type (should create new, not update)
	reaction2, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "❤️",
	})
	require.NoError(t, err)

	// Both reactions should exist
	require.NotEqual(t, reaction1.ID, reaction2.ID)

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{
		NoteID: &note.ID,
	})
	require.NoError(t, err)
	require.Len(t, reactions, 2)

	ts.Close()
}

func TestReactionDeletedWithNote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-delete-with-note")
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)

	require.NoError(t, ts.DeleteNote(ctx, &store.DeleteNote{ID: note.ID}))
	got, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reaction.ID})
	require.NoError(t, err)
	require.Nil(t, got)

	ts.Close()
}

func TestReactionInsertedAfterNoteCleanupStartsIsRemoved(t *testing.T) {
	setupContext := context.Background()
	barrierDriver := &noteDeleteBarrierDriver{
		deleteStarted:  make(chan struct{}),
		continueDelete: make(chan struct{}),
	}
	ts := newReactionTestingStoreWithDriver(setupContext, t, func(driver store.Driver) store.Driver {
		barrierDriver.Driver = driver
		return barrierDriver
	})
	ctx, cancel := context.WithTimeout(setupContext, 10*time.Second)
	defer cancel()

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-insert-during-delete")

	deleteResult := make(chan error, 1)
	go func() {
		deleteResult <- ts.DeleteNote(ctx, &store.DeleteNote{ID: note.ID})
	}()
	deleteReleased := false
	defer func() {
		if !deleteReleased {
			close(barrierDriver.continueDelete)
		}
	}()

	select {
	case <-barrierDriver.deleteStarted:
	case err := <-deleteResult:
		require.FailNowf(t, "note delete returned before reaching barrier", "error: %v", err)
	case <-ctx.Done():
		require.FailNow(t, "timed out waiting for note delete barrier")
	}

	// The deletion transaction has not started yet. A reaction created in this
	// window must still be removed by the compound note deletion.
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		NoteID:       note.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)
	require.NotNil(t, reaction)

	close(barrierDriver.continueDelete)
	deleteReleased = true
	select {
	case err := <-deleteResult:
		require.NoError(t, err)
	case <-ctx.Done():
		require.FailNow(t, "timed out waiting for note delete")
	}

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{NoteID: &note.ID})
	require.NoError(t, err)
	require.Empty(t, reactions)
}

func TestReactionUpsertBlocksWhileNoteDeleteIsUncommitted(t *testing.T) {
	setupContext := context.Background()
	ts := NewTestingStore(setupContext, t)
	t.Cleanup(func() {
		require.NoError(t, ts.Close())
	})
	ctx, cancel := context.WithTimeout(setupContext, 10*time.Second)
	defer cancel()

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	note := createReactionTestNote(ctx, t, ts, user.ID, "reaction-waits-for-delete")

	dbTx, err := ts.GetDriver().GetDB().BeginTx(ctx, nil)
	require.NoError(t, err)
	txOpen := true
	defer func() {
		if txOpen {
			_ = dbTx.Rollback()
		}
	}()

	deleteNoteQuery := "DELETE FROM note WHERE id = ?"
	if getDriverFromEnv() == "postgres" {
		deleteNoteQuery = "DELETE FROM note WHERE id = $1"
	}
	_, err = dbTx.ExecContext(ctx, deleteNoteQuery, note.ID)
	require.NoError(t, err)

	var reaction *store.Reaction
	var upsertErr error
	if getDriverFromEnv() == "sqlite" {
		type upsertResult struct {
			reaction *store.Reaction
			err      error
		}
		resultChannel := make(chan upsertResult, 1)
		go func() {
			reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
				CreatorID:    user.ID,
				NoteID:       note.ID,
				ReactionType: "👍",
			})
			resultChannel <- upsertResult{reaction: reaction, err: err}
		}()

		poll := time.NewTicker(time.Millisecond)
		defer poll.Stop()
		for ts.GetDriver().GetDB().Stats().InUse < 2 {
			select {
			case result := <-resultChannel:
				require.FailNowf(t, "reaction upsert completed before note delete committed", "reaction: %+v, error: %v", result.reaction, result.err)
			case <-poll.C:
			case <-ctx.Done():
				require.FailNow(t, "timed out waiting for reaction upsert to reach SQLite")
			}
		}

		require.NoError(t, dbTx.Commit())
		txOpen = false
		select {
		case result := <-resultChannel:
			reaction, upsertErr = result.reaction, result.err
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for SQLite reaction upsert")
		}
	} else {
		// Both client/server database engines must wait for the uncommitted
		// parent deletion before deciding whether the note still exists.
		lockWaitContext, lockWaitCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		startTime := time.Now()
		reaction, upsertErr = ts.UpsertReaction(lockWaitContext, &store.Reaction{
			CreatorID:    user.ID,
			NoteID:       note.ID,
			ReactionType: "👍",
		})
		lockWaitCancel()
		require.Error(t, upsertErr)
		require.Nil(t, reaction)
		require.GreaterOrEqual(t, time.Since(startTime), 450*time.Millisecond, "reaction upsert must wait for the note delete transaction")

		require.NoError(t, dbTx.Commit())
		txOpen = false
		reaction, upsertErr = ts.UpsertReaction(ctx, &store.Reaction{
			CreatorID:    user.ID,
			NoteID:       note.ID,
			ReactionType: "👍",
		})
	}
	require.ErrorIs(t, upsertErr, store.ErrReactionNoteNotFound)
	require.Nil(t, reaction)

	reactions, err := ts.ListReactions(ctx, &store.FindReaction{NoteID: &note.ID})
	require.NoError(t, err)
	require.Empty(t, reactions)
}

type noteDeleteBarrierDriver struct {
	store.Driver
	deleteStarted  chan struct{}
	continueDelete chan struct{}
}

func (d *noteDeleteBarrierDriver) DeleteNoteWithPolicy(ctx context.Context, delete *store.DeleteNoteWithPolicy) (*store.DeleteNoteWithPolicyResult, error) {
	close(d.deleteStarted)
	select {
	case <-d.continueDelete:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return d.Driver.DeleteNoteWithPolicy(ctx, delete)
}

func newReactionTestingStoreWithDriver(ctx context.Context, t *testing.T, wrap func(store.Driver) store.Driver) *store.Store {
	t.Helper()
	testingProfile := getTestingProfileForDriver(t, getDriverFromEnv())
	databaseDriver, err := db.NewDBDriver(testingProfile)
	require.NoError(t, err)

	ts := store.New(wrap(databaseDriver), testingProfile)
	require.NoError(t, ts.Migrate(ctx))
	t.Cleanup(func() {
		require.NoError(t, ts.Close())
	})
	return ts
}

func createReactionTestNote(ctx context.Context, t *testing.T, ts *store.Store, creatorID int32, uid string) *store.Note {
	t.Helper()
	note, err := ts.CreateNote(ctx, &store.Note{
		UID:        uid,
		CreatorID:  creatorID,
		Content:    uid,
		Visibility: store.Private,
	})
	require.NoError(t, err)
	return note
}

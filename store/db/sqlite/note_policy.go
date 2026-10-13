package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

// readSQLiteNoteActor reads the actor's lifecycle state and instance role
// inside the mutation transaction. A missing user is the zero state.
func readSQLiteNoteActor(ctx context.Context, executor dbExecutor, userID int32) (store.NoteActorState, error) {
	var rowStatus store.RowStatus
	var role store.Role
	err := executor.QueryRowContext(ctx, "SELECT row_status, role FROM user WHERE id = ?", userID).Scan(&rowStatus, &role)
	if stderrors.Is(err, sql.ErrNoRows) {
		return store.NoteActorState{}, nil
	}
	if err != nil {
		return store.NoteActorState{}, err
	}
	return store.NewNoteActorState(rowStatus, role), nil
}

// requireSQLiteActiveNoteActor resolves an actor that must be active;
// anything else is a permission denial.
func requireSQLiteActiveNoteActor(ctx context.Context, executor dbExecutor, userID int32) (store.NoteActorState, error) {
	actor, err := readSQLiteNoteActor(ctx, executor, userID)
	if err != nil {
		return store.NoteActorState{}, err
	}
	if !actor.Active {
		return store.NoteActorState{}, store.ErrNotePermissionDenied
	}
	return actor, nil
}

// validateSQLiteNoteWritePolicy authorizes a transport-facing note mutation
// against current database state and returns the validated snapshot.
func validateSQLiteNoteWritePolicy(ctx context.Context, executor dbExecutor, noteID int32, policy *store.NoteWritePolicy, update *store.UpdateNote) (*store.NoteWriteSnapshot, error) {
	actor, err := readSQLiteNoteActor(ctx, executor, policy.ActorUserID)
	if err != nil {
		return nil, err
	}
	if !actor.Active {
		return nil, store.ErrNoteSpaceMembershipRequired
	}

	snapshot := &store.NoteWriteSnapshot{ActorIsAdmin: actor.Admin}
	var spaceID sql.NullInt64
	if err := executor.QueryRowContext(ctx, `SELECT creator_id, row_status, space_id, visibility FROM note WHERE id = ?`, noteID).Scan(
		&snapshot.CreatorID, &snapshot.RowStatus, &spaceID, &snapshot.Visibility,
	); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNoteMutationConflict
		}
		return nil, err
	}
	snapshot.SpaceID = store.NullInt32Pointer(spaceID)
	if err := populateSQLiteNotePolicySpaceState(ctx, executor, policy.ActorUserID, actor, update, snapshot); err != nil {
		return nil, err
	}

	if update != nil && update.Visibility != nil && *update.Visibility == store.SpaceAudience {
		var shareID int32
		err := executor.QueryRowContext(ctx, `SELECT id FROM note_share
			WHERE note_id = ? AND (expires_ts IS NULL OR expires_ts > CAST(strftime('%s', 'now') AS INTEGER))
			LIMIT 1`, noteID).Scan(&shareID)
		snapshot.HasActiveShare = err == nil
		if err != nil && !stderrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if err := store.ValidateNoteWriteSnapshot(policy, update, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func populateSQLiteNotePolicySpaceState(
	ctx context.Context,
	executor dbExecutor,
	actorUserID int32,
	actor store.NoteActorState,
	update *store.UpdateNote,
	snapshot *store.NoteWriteSnapshot,
) error {
	if snapshot.SpaceID != nil {
		exists, member, err := sqliteNotePolicySpaceState(ctx, executor, *snapshot.SpaceID, actorUserID, actor)
		if err != nil {
			return err
		}
		snapshot.SourceSpaceExists = exists
		snapshot.SourceMemberActive = member
	}
	if update != nil && update.SpaceID != nil {
		exists, member, err := sqliteNotePolicySpaceState(ctx, executor, *update.SpaceID, actorUserID, actor)
		if err != nil {
			return err
		}
		snapshot.TargetSpaceExists = exists
		snapshot.TargetMemberActive = member
	}
	return nil
}

// sqliteNotePolicySpaceState reports whether the Space exists and whether the
// actor is an active member. Membership is not consulted for an instance
// administrator, so it is not queried.
func sqliteNotePolicySpaceState(ctx context.Context, executor dbExecutor, spaceID, actorUserID int32, actor store.NoteActorState) (bool, bool, error) {
	exists, err := sqliteSpaceExists(ctx, executor, spaceID)
	if err != nil || !exists || actor.Admin {
		return exists, false, err
	}
	member, err := sqliteSpaceMemberActive(ctx, executor, spaceID, actorUserID)
	return true, member, err
}

func sqliteSpaceExists(ctx context.Context, executor dbExecutor, spaceID int32) (bool, error) {
	var exists bool
	if err := executor.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM space WHERE id = ?)", spaceID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

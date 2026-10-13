package mysql

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

// readMySQLNoteActor reads the actor's lifecycle state and instance role
// inside the mutation transaction. A missing user is the zero state.
func readMySQLNoteActor(ctx context.Context, tx *sql.Tx, userID int32) (store.NoteActorState, error) {
	var rowStatus store.RowStatus
	var role store.Role
	err := tx.QueryRowContext(ctx, "SELECT row_status, role FROM user WHERE id = ?", userID).Scan(&rowStatus, &role)
	if stderrors.Is(err, sql.ErrNoRows) {
		return store.NoteActorState{}, nil
	}
	if err != nil {
		return store.NoteActorState{}, err
	}
	return store.NewNoteActorState(rowStatus, role), nil
}

// requireMySQLActiveNoteActor resolves an actor that must be active; anything
// else is a permission denial.
func requireMySQLActiveNoteActor(ctx context.Context, tx *sql.Tx, userID int32) (store.NoteActorState, error) {
	actor, err := readMySQLNoteActor(ctx, tx, userID)
	if err != nil {
		return store.NoteActorState{}, err
	}
	if !actor.Active {
		return store.NoteActorState{}, store.ErrNotePermissionDenied
	}
	return actor, nil
}

// validateMySQLNoteWritePolicy authorizes a transport-facing note mutation
// against current database state and returns the validated snapshot.
func validateMySQLNoteWritePolicy(ctx context.Context, tx *sql.Tx, noteID int32, policy *store.NoteWritePolicy, update *store.UpdateNote) (*store.NoteWriteSnapshot, error) {
	actor, err := readMySQLNoteActor(ctx, tx, policy.ActorUserID)
	if err != nil {
		return nil, err
	}
	if !actor.Active {
		return nil, store.ErrNoteSpaceMembershipRequired
	}

	snapshot := &store.NoteWriteSnapshot{ActorIsAdmin: actor.Admin}
	var spaceID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT creator_id, row_status, space_id, visibility FROM note WHERE id = ?`, noteID).Scan(
		&snapshot.CreatorID, &snapshot.RowStatus, &spaceID, &snapshot.Visibility,
	); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNoteMutationConflict
		}
		return nil, err
	}
	snapshot.SpaceID = store.NullInt32Pointer(spaceID)
	if snapshot.SpaceID != nil {
		var err error
		snapshot.SourceSpaceExists, snapshot.SourceMemberActive, err = mysqlNotePolicySpaceState(ctx, tx, *snapshot.SpaceID, policy.ActorUserID, actor)
		if err != nil {
			return nil, err
		}
	}
	if update != nil && update.SpaceID != nil {
		var err error
		snapshot.TargetSpaceExists, snapshot.TargetMemberActive, err = mysqlNotePolicySpaceState(ctx, tx, *update.SpaceID, policy.ActorUserID, actor)
		if err != nil {
			return nil, err
		}
	}

	if update != nil && update.Visibility != nil && *update.Visibility == store.SpaceAudience {
		var shareID int32
		err := tx.QueryRowContext(ctx, `SELECT id FROM note_share
			WHERE note_id = ? AND (expires_ts IS NULL OR expires_ts > UNIX_TIMESTAMP())
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

// mysqlNotePolicySpaceState reports whether the Space exists and whether the
// actor is an active member. Membership is not consulted for an instance
// administrator, so it is not queried.
func mysqlNotePolicySpaceState(ctx context.Context, tx *sql.Tx, spaceID, actorUserID int32, actor store.NoteActorState) (bool, bool, error) {
	exists, err := mysqlSpaceExists(ctx, tx, spaceID)
	if err != nil || !exists || actor.Admin {
		return exists, false, err
	}
	member, err := mysqlSpaceMemberActive(ctx, tx, spaceID, actorUserID)
	return true, member, err
}

func mysqlSpaceExists(ctx context.Context, tx *sql.Tx, spaceID int32) (bool, error) {
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM space WHERE id = ?)", spaceID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

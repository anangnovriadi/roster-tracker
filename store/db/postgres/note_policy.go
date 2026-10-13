package postgres

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

// readPostgresNoteActor reads the actor's lifecycle state and instance role
// inside the mutation transaction. A missing user is the zero state.
func readPostgresNoteActor(ctx context.Context, tx *sql.Tx, userID int32) (store.NoteActorState, error) {
	var rowStatus store.RowStatus
	var role store.Role
	err := tx.QueryRowContext(ctx, `SELECT row_status, role FROM "user" WHERE id = $1`, userID).Scan(&rowStatus, &role)
	if stderrors.Is(err, sql.ErrNoRows) {
		return store.NoteActorState{}, nil
	}
	if err != nil {
		return store.NoteActorState{}, err
	}
	return store.NewNoteActorState(rowStatus, role), nil
}

// requirePostgresActiveNoteActor resolves an actor that must be active;
// anything else is a permission denial.
func requirePostgresActiveNoteActor(ctx context.Context, tx *sql.Tx, userID int32) (store.NoteActorState, error) {
	actor, err := readPostgresNoteActor(ctx, tx, userID)
	if err != nil {
		return store.NoteActorState{}, err
	}
	if !actor.Active {
		return store.NoteActorState{}, store.ErrNotePermissionDenied
	}
	return actor, nil
}

// validatePostgresNoteWritePolicy authorizes a transport-facing note mutation
// against current database state and returns the validated snapshot.
func validatePostgresNoteWritePolicy(ctx context.Context, tx *sql.Tx, noteID int32, policy *store.NoteWritePolicy, update *store.UpdateNote) (*store.NoteWriteSnapshot, error) {
	actor, err := readPostgresNoteActor(ctx, tx, policy.ActorUserID)
	if err != nil {
		return nil, err
	}
	if !actor.Active {
		return nil, store.ErrNoteSpaceMembershipRequired
	}

	snapshot := &store.NoteWriteSnapshot{ActorIsAdmin: actor.Admin}
	var sourceSpace sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT creator_id, row_status, space_id, visibility FROM note WHERE id = $1`, noteID).Scan(
		&snapshot.CreatorID, &snapshot.RowStatus, &sourceSpace, &snapshot.Visibility,
	); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNoteMutationConflict
		}
		return nil, err
	}
	snapshot.SpaceID = store.NullInt32Pointer(sourceSpace)
	if err := populatePostgresNotePolicySpaceState(ctx, tx, policy.ActorUserID, update, snapshot); err != nil {
		return nil, err
	}

	if update != nil && update.Visibility != nil && *update.Visibility == store.SpaceAudience {
		var shareID int32
		err := tx.QueryRowContext(ctx, `SELECT id FROM note_share
			WHERE note_id = $1 AND (expires_ts IS NULL OR expires_ts > EXTRACT(EPOCH FROM NOW()))
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

func populatePostgresNotePolicySpaceState(
	ctx context.Context,
	tx *sql.Tx,
	actorUserID int32,
	update *store.UpdateNote,
	snapshot *store.NoteWriteSnapshot,
) error {
	if snapshot.SpaceID != nil {
		var err error
		snapshot.SourceSpaceExists, snapshot.SourceMemberActive, err = readPostgresNoteSpaceState(ctx, tx, *snapshot.SpaceID, actorUserID)
		if err != nil {
			return err
		}
	}
	if update != nil && update.SpaceID != nil {
		if snapshot.SpaceID != nil && *snapshot.SpaceID == *update.SpaceID {
			snapshot.TargetSpaceExists = snapshot.SourceSpaceExists
			snapshot.TargetMemberActive = snapshot.SourceMemberActive
			return nil
		}
		var err error
		snapshot.TargetSpaceExists, snapshot.TargetMemberActive, err = readPostgresNoteSpaceState(ctx, tx, *update.SpaceID, actorUserID)
		if err != nil {
			return err
		}
	}
	return nil
}

// readPostgresNoteSpaceState reports whether the Space exists and whether the
// actor is an active member, in one round trip.
func readPostgresNoteSpaceState(ctx context.Context, tx *sql.Tx, spaceID, actorUserID int32) (bool, bool, error) {
	var spaceExists, memberActive bool
	if err := tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM space WHERE id = $1),
		EXISTS(SELECT 1 FROM space_member WHERE space_id = $1 AND user_id = $2
			AND status = 'ACTIVE' AND role IN ('ADMIN', 'USER'))`,
		spaceID, actorUserID).Scan(&spaceExists, &memberActive); err != nil {
		return false, false, err
	}
	return spaceExists, memberActive, nil
}

func postgresSpaceExists(ctx context.Context, tx *sql.Tx, spaceID int32) (bool, error) {
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM space WHERE id = $1)", spaceID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

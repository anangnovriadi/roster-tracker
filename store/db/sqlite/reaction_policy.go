package sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func validateSQLiteReactionWritePolicy(ctx context.Context, tx dbExecutor, reaction *store.Reaction) error {
	policy := reaction.Policy
	participation, err := loadSQLiteNoteParticipation(ctx, tx, reaction.NoteID, policy.ActorUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrReactionNoteNotFound
		}
		return errors.Wrap(err, "failed to read reaction participation")
	}
	return store.ValidateReactionWriteParticipation(reaction, participation)
}

// validateSQLiteReactionDeletePolicy authorizes a withdrawal and returns the
// participation snapshot so the caller can honour an administrator actor.
func validateSQLiteReactionDeletePolicy(ctx context.Context, tx dbExecutor, reaction *store.Reaction) (*store.NoteCommentAuthorizationSnapshot, error) {
	policy := reaction.Policy
	participation, err := loadSQLiteNoteParticipation(ctx, tx, reaction.NoteID, policy.ActorUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrReactionNoteNotFound
		}
		return nil, errors.Wrap(err, "failed to read reaction participation")
	}
	if err := store.ValidateReactionWithdrawal(reaction, participation); err != nil {
		return nil, err
	}
	return participation, nil
}

func sqliteSpaceMemberActive(ctx context.Context, tx dbExecutor, spaceID, userID int32) (bool, error) {
	var role store.SpaceMemberRole
	err := tx.QueryRowContext(ctx, `SELECT role FROM space_member
		WHERE space_id = ? AND user_id = ? AND status = 'ACTIVE' AND role IN ('ADMIN', 'USER')`, spaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return role.IsActiveMember(), nil
}

package postgres

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func authorizePostgresNoteComment(ctx context.Context, tx *sql.Tx, contextNoteID, actorUserID int32) error {
	snapshot, err := readPostgresNoteParticipation(ctx, tx, contextNoteID, actorUserID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) || stderrors.Is(err, store.ErrNoteMutationConflict) {
			return store.ErrNoteSpaceNotWritable
		}
		return err
	}
	return store.ValidateNoteCommentAuthorization(snapshot)
}

// readPostgresNoteParticipation resolves the current actor, Space, membership,
// and note state shared by comment and reaction participation.
func readPostgresNoteParticipation(ctx context.Context, tx *sql.Tx, noteID, actorUserID int32) (*store.NoteCommentAuthorizationSnapshot, error) {
	actor, err := readPostgresNoteActor(ctx, tx, actorUserID)
	if err != nil {
		return nil, err
	}

	snapshot := &store.NoteCommentAuthorizationSnapshot{
		ActorUserID: actorUserID,
		Actor:       actor,
		ContextID:   noteID,
	}
	var contextSpace sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT creator_id, row_status, visibility, space_id
		FROM note WHERE id = $1`, noteID).Scan(
		&snapshot.ContextCreatorID, &snapshot.ContextRowStatus, &snapshot.ContextVisibility, &contextSpace,
	)
	if err != nil {
		return nil, err
	}
	snapshot.ContextSpaceID = store.NullInt32Pointer(contextSpace)
	if snapshot.ContextSpaceID != nil {
		if err := tx.QueryRowContext(ctx, `SELECT
			EXISTS(SELECT 1 FROM space WHERE id = $1),
			EXISTS(SELECT 1 FROM space_member WHERE space_id = $1 AND user_id = $2
				AND status = 'ACTIVE' AND role IN ('ADMIN', 'USER'))`,
			*snapshot.ContextSpaceID, actorUserID).Scan(&snapshot.ContextSpaceExists, &snapshot.ContextMemberActive); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

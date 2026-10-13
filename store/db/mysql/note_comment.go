package mysql

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func authorizeMySQLNoteComment(ctx context.Context, tx *sql.Tx, contextNoteID, actorUserID int32) error {
	snapshot, err := loadMySQLNoteParticipation(ctx, tx, contextNoteID, actorUserID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) || stderrors.Is(err, store.ErrNoteMutationConflict) {
			return store.ErrNoteSpaceNotWritable
		}
		return err
	}
	return store.ValidateNoteCommentAuthorization(snapshot)
}

// loadMySQLNoteParticipation resolves the current actor, Space, membership,
// and note state shared by comment and reaction participation.
func loadMySQLNoteParticipation(ctx context.Context, tx *sql.Tx, noteID, actorUserID int32) (*store.NoteCommentAuthorizationSnapshot, error) {
	actor, err := readMySQLNoteActor(ctx, tx, actorUserID)
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
		FROM note WHERE id = ?`, noteID).Scan(
		&snapshot.ContextCreatorID, &snapshot.ContextRowStatus, &snapshot.ContextVisibility, &contextSpace,
	)
	if err != nil {
		return nil, err
	}
	snapshot.ContextSpaceID = store.NullInt32Pointer(contextSpace)
	if snapshot.ContextSpaceID != nil {
		snapshot.ContextSpaceExists, err = mysqlSpaceExists(ctx, tx, *snapshot.ContextSpaceID)
		if err != nil {
			return nil, err
		}
		if snapshot.ContextSpaceExists && !actor.Admin {
			snapshot.ContextMemberActive, err = mysqlSpaceMemberActive(ctx, tx, *snapshot.ContextSpaceID, actorUserID)
			if err != nil {
				return nil, err
			}
		}
	}
	return snapshot, nil
}

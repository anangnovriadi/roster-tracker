package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/anangnovriadi/roster-tracker/store"
)

func authorizeSQLiteNoteComment(ctx context.Context, tx dbExecutor, contextNoteID, actorUserID int32) error {
	snapshot, err := loadSQLiteNoteParticipation(ctx, tx, contextNoteID, actorUserID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return store.ErrNoteSpaceNotWritable
		}
		return err
	}
	return store.ValidateNoteCommentAuthorization(snapshot)
}

// loadSQLiteNoteParticipation resolves the current actor, Space, membership,
// and note state shared by comment and reaction participation.
func loadSQLiteNoteParticipation(ctx context.Context, tx dbExecutor, noteID, actorUserID int32) (*store.NoteCommentAuthorizationSnapshot, error) {
	actor, err := readSQLiteNoteActor(ctx, tx, actorUserID)
	if err != nil {
		return nil, err
	}
	snapshot := &store.NoteCommentAuthorizationSnapshot{ActorUserID: actorUserID, Actor: actor, ContextID: noteID}

	var contextSpace sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT creator_id, row_status, visibility, space_id FROM note WHERE id = ?`, noteID).Scan(
		&snapshot.ContextCreatorID, &snapshot.ContextRowStatus, &snapshot.ContextVisibility, &contextSpace,
	)
	if err != nil {
		return nil, err
	}
	snapshot.ContextSpaceID = store.NullInt32Pointer(contextSpace)

	if snapshot.ContextSpaceID != nil {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM space WHERE id = ?)", *snapshot.ContextSpaceID).Scan(&exists); err == nil {
			snapshot.ContextSpaceExists = exists
		} else if !stderrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if actor.Admin {
			return snapshot, nil
		}
		if snapshot.ContextMemberActive, err = sqliteSpaceMemberActive(ctx, tx, *snapshot.ContextSpaceID, actorUserID); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"
	"slices"

	"github.com/anangnovriadi/roster-tracker/store"
)

type sqliteRelationNoteState struct {
	id         int32
	creatorID  int32
	rowStatus  store.RowStatus
	visibility store.Visibility
	spaceID    *int32
}

func validateSQLiteNoteRelationEndpoints(ctx context.Context, executor dbExecutor, mutation *store.NoteMutation) error {
	if !mutation.ReplaceReferenceRelations && mutation.CommentContextNoteID == nil {
		return nil
	}

	actorUserID := mutation.ActorUserID()
	actor, err := readSQLiteNoteActor(ctx, executor, actorUserID)
	if err != nil {
		return err
	}

	endpointIDs := sqliteRelationAuthorizationEndpointIDs(mutation)
	for _, endpointID := range endpointIDs {
		state, err := getSQLiteRelationNoteState(ctx, executor, endpointID)
		if err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				return store.ErrNoteMutationConflict
			}
			return err
		}
		snapshot := &store.NoteRelationEndpointSnapshot{
			ActorUserID:        actorUserID,
			Actor:              actor,
			EndpointID:         state.id,
			EndpointCreatorID:  state.creatorID,
			EndpointRowStatus:  state.rowStatus,
			EndpointVisibility: state.visibility,
			EndpointSpaceID:    state.spaceID,
		}
		if state.spaceID != nil {
			if err := executor.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM space WHERE id = ?)", *state.spaceID).Scan(&snapshot.EndpointSpaceExists); err != nil {
				return err
			}
			if snapshot.EndpointSpaceExists && !actor.Admin {
				active, err := sqliteSpaceMemberActive(ctx, executor, *state.spaceID, actorUserID)
				if err != nil {
					return err
				}
				snapshot.EndpointMemberActive = active
			}
		}
		if err := store.ValidateNoteRelationEndpointRead(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func getSQLiteRelationNoteState(ctx context.Context, executor dbExecutor, noteID int32) (*sqliteRelationNoteState, error) {
	state := &sqliteRelationNoteState{}
	var spaceID sql.NullInt64
	if err := executor.QueryRowContext(ctx, `SELECT id, creator_id, row_status, visibility, space_id
		FROM note WHERE id = ?`, noteID).Scan(
		&state.id, &state.creatorID, &state.rowStatus, &state.visibility, &spaceID,
	); err != nil {
		return nil, err
	}
	state.spaceID = store.NullInt32Pointer(spaceID)
	return state, nil
}

func sqliteRelationAuthorizationEndpointIDs(mutation *store.NoteMutation) []int32 {
	unique := make(map[int32]struct{}, len(mutation.ReferenceRelations)+1)
	if mutation.NoteCreate == nil && mutation.NoteID > 0 {
		unique[mutation.NoteID] = struct{}{}
	} else if mutation.CommentContextNoteID != nil {
		unique[*mutation.CommentContextNoteID] = struct{}{}
	}
	for _, relation := range mutation.ReferenceRelations {
		if relation != nil {
			unique[relation.RelatedNoteID] = struct{}{}
		}
	}
	ids := make([]int32, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

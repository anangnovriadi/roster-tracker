package api

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func (s *APIService) listUsersByID(ctx context.Context, userIDs []int32) (map[int32]*store.User, error) {
	if len(userIDs) == 0 {
		return map[int32]*store.User{}, nil
	}

	uniqueUserIDs := make([]int32, 0, len(userIDs))
	seenUserIDs := make(map[int32]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if _, seen := seenUserIDs[userID]; seen {
			continue
		}
		seenUserIDs[userID] = struct{}{}
		uniqueUserIDs = append(uniqueUserIDs, userID)
	}

	users, err := s.Store.ListUsers(ctx, &store.FindUser{IDList: uniqueUserIDs})
	if err != nil {
		return nil, err
	}

	usersByID := make(map[int32]*store.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}
	return usersByID, nil
}

func (s *APIService) listUsernamesByID(ctx context.Context, userIDs []int32) (map[int32]string, error) {
	usersByID, err := s.listUsersByID(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	usernamesByID := make(map[int32]string, len(usersByID))
	for _, user := range usersByID {
		usernamesByID[user.ID] = user.Username
	}
	return usernamesByID, nil
}

func (s *APIService) ListUserStats(ctx context.Context, request *apipb.ListUserStatsRequest) (*apipb.ListUserStatsResponse, error) {
	// users/- lists every user; users/{user} narrows the list to one creator.
	var creatorID *int32
	if request.Parent != BuildUserName("-") {
		user, err := ResolveUserByName(ctx, s.Store, request.Parent)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid parent: %v", err)
		}
		if user == nil {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		creatorID = &user.ID
	}

	rowStatus := convertStateToStore(request.State)
	noteFind := &store.FindNote{
		CreatorID: creatorID,
		// Exclude comments by default.
		ExcludeComments: true,
		ExcludeContent:  true,
		RowStatus:       &rowStatus,
	}

	accessScope, currentUser, err := s.resolveNoteAccessScope(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	if request.Filter != "" {
		if err := s.validateNoteFilterForUser(ctx, request.Filter, currentUser); err != nil {
			return nil, err
		}
		noteFind.Filters = append(noteFind.Filters, request.Filter)
	}

	if request.State == apipb.State_ARCHIVED {
		// Archived notes are only visible to their creator.
		if currentUser == nil || (creatorID != nil && *creatorID != currentUser.ID) {
			return &apipb.ListUserStatsResponse{}, nil
		}
		noteFind.CreatorID = &currentUser.ID
	}
	noteFind.Access = accessScope

	userNoteStatMap := make(map[int32]*apipb.UserStats)
	pinnedNoteUIDsByUserID := make(map[int32][]string)
	limit := 1000
	offset := 0
	noteFind.Limit = &limit
	noteFind.Offset = &offset

	for {
		notes, err := s.Store.ListNotes(ctx, noteFind)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list notes: %v", err)
		}
		if len(notes) == 0 {
			break
		}

		for _, note := range notes {
			// Initialize user stats if not exists
			if _, exists := userNoteStatMap[note.CreatorID]; !exists {
				userNoteStatMap[note.CreatorID] = &apipb.UserStats{
					Name:                  "",
					TagCounts:             make(map[string]int32),
					NoteCreatedTimestamps: []*timestamppb.Timestamp{},
					NoteUpdatedTimestamps: []*timestamppb.Timestamp{},
					PinnedNotes:           []string{},
					NoteTypeStats: &apipb.UserStats_NoteTypeStats{
						LinkCount: 0,
						CodeCount: 0,
						TodoCount: 0,
						UndoCount: 0,
					},
				}
			}

			stats := userNoteStatMap[note.CreatorID]

			stats.NoteCreatedTimestamps = append(stats.NoteCreatedTimestamps, timestamppb.New(time.Unix(note.CreatedTs, 0)))
			stats.NoteUpdatedTimestamps = append(stats.NoteUpdatedTimestamps, timestamppb.New(time.Unix(note.UpdatedTs, 0)))

			// Count note stats
			stats.TotalNoteCount++

			// Count tags and other properties
			if note.Payload != nil {
				incrementTagCounts(stats.TagCounts, note.Payload.Tags)
				if note.Payload.Property != nil {
					if note.Payload.Property.HasLink {
						stats.NoteTypeStats.LinkCount++
					}
					if note.Payload.Property.HasCode {
						stats.NoteTypeStats.CodeCount++
					}
					if note.Payload.Property.HasTaskList {
						stats.NoteTypeStats.TodoCount++
					}
					if note.Payload.Property.HasIncompleteTasks {
						stats.NoteTypeStats.UndoCount++
					}
				}
			}

			// Track pinned notes
			if note.Pinned {
				pinnedNoteUIDsByUserID[note.CreatorID] = append(pinnedNoteUIDsByUserID[note.CreatorID], note.UID)
			}
		}

		offset += limit
	}

	userNoteStats := []*apipb.UserStats{}
	userIDs := make([]int32, 0, len(userNoteStatMap))
	for userID := range userNoteStatMap {
		userIDs = append(userIDs, userID)
	}
	usernamesByID, err := s.listUsernamesByID(ctx, userIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}
	for userID, userNoteStat := range userNoteStatMap {
		username, ok := usernamesByID[userID]
		if !ok {
			return nil, status.Errorf(codes.Internal, "failed to resolve user stats name")
		}
		userNoteStat.Name = fmt.Sprintf("%s/stats", BuildUserName(username))
		for _, noteUID := range pinnedNoteUIDsByUserID[userID] {
			userNoteStat.PinnedNotes = append(userNoteStat.PinnedNotes, NoteNamePrefix+noteUID)
		}
		userNoteStats = append(userNoteStats, userNoteStat)
	}

	response := &apipb.ListUserStatsResponse{
		UserStats: userNoteStats,
	}
	return response, nil
}

func (s *APIService) GetUserStats(ctx context.Context, request *apipb.GetUserStatsRequest) (*apipb.UserStats, error) {
	user, err := ResolveUserByName(ctx, s.Store, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	userID := user.ID

	accessScope, currentUser, err := s.resolveNoteAccessScope(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	normalStatus := store.Normal
	noteFind := &store.FindNote{
		CreatorID: &userID,
		// Exclude comments by default.
		ExcludeComments: true,
		ExcludeContent:  true,
		RowStatus:       &normalStatus,
	}

	if request.Filter != "" {
		if err := s.validateNoteFilterForUser(ctx, request.Filter, currentUser); err != nil {
			return nil, err
		}
		noteFind.Filters = append(noteFind.Filters, request.Filter)
	}

	noteFind.Access = accessScope

	createdTimestamps := []*timestamppb.Timestamp{}
	updatedTimestamps := []*timestamppb.Timestamp{}
	tagCount := make(map[string]int32)
	linkCount := int32(0)
	codeCount := int32(0)
	todoCount := int32(0)
	undoCount := int32(0)
	pinnedNotes := []string{}
	totalNoteCount := int32(0)

	limit := 1000
	offset := 0
	noteFind.Limit = &limit
	noteFind.Offset = &offset

	for {
		notes, err := s.Store.ListNotes(ctx, noteFind)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list notes: %v", err)
		}
		if len(notes) == 0 {
			break
		}

		totalNoteCount += int32(len(notes))

		for _, note := range notes {
			createdTimestamps = append(createdTimestamps, timestamppb.New(time.Unix(note.CreatedTs, 0)))
			updatedTimestamps = append(updatedTimestamps, timestamppb.New(time.Unix(note.UpdatedTs, 0)))
			// Count different note types based on content.
			if note.Payload != nil {
				incrementTagCounts(tagCount, note.Payload.Tags)
				if note.Payload.Property != nil {
					if note.Payload.Property.HasLink {
						linkCount++
					}
					if note.Payload.Property.HasCode {
						codeCount++
					}
					if note.Payload.Property.HasTaskList {
						todoCount++
					}
					if note.Payload.Property.HasIncompleteTasks {
						undoCount++
					}
				}
			}
			if note.Pinned {
				pinnedNotes = append(pinnedNotes, NoteNamePrefix+note.UID)
			}
		}

		offset += limit
	}

	userStats := &apipb.UserStats{
		Name:                  fmt.Sprintf("%s/stats", BuildUserName(user.Username)),
		NoteCreatedTimestamps: createdTimestamps,
		NoteUpdatedTimestamps: updatedTimestamps,
		TagCounts:             tagCount,
		PinnedNotes:           pinnedNotes,
		TotalNoteCount:        totalNoteCount,
		NoteTypeStats: &apipb.UserStats_NoteTypeStats{
			LinkCount: linkCount,
			CodeCount: codeCount,
			TodoCount: todoCount,
			UndoCount: undoCount,
		},
	}

	if currentUser != nil && currentUser.ID == userID {
		size, err := s.Store.GetAttachmentStorageUsage(ctx, userID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get attachment storage usage: %v", err)
		}
		userStats.AttachmentStorageBytes = &size
	}

	return userStats, nil
}

// incrementTagCounts counts each distinct tag at most once for one note payload.
func incrementTagCounts(counts map[string]int32, tags []string) {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		counts[tag]++
	}
}

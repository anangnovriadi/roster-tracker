package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestUpdateNoteTimestamps(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	user, err := ts.CreateRegularUser(ctx, "timestamp-owner")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	originalCreate := int64(1577836800)
	originalUpdate := originalCreate + 3600
	customCreate := originalCreate - 86400
	customUpdate := originalUpdate - 86400

	for _, tc := range []struct {
		name       string
		paths      []string
		patch      *apipb.Note
		wantCreate int64
		wantUpdate int64
		autoUpdate bool
	}{
		{"creation only", []string{"create_time"}, &apipb.Note{CreateTime: &timestamppb.Timestamp{Seconds: customCreate}}, customCreate, originalUpdate, false},
		{"modification only", []string{"update_time"}, &apipb.Note{UpdateTime: &timestamppb.Timestamp{Seconds: customUpdate}}, originalCreate, customUpdate, false},
		{"both", []string{"create_time", "update_time"}, &apipb.Note{CreateTime: &timestamppb.Timestamp{Seconds: customCreate}, UpdateTime: &timestamppb.Timestamp{Seconds: customUpdate}}, customCreate, customUpdate, false},
		{"with content", []string{"content", "create_time", "update_time"}, &apipb.Note{Content: "changed", CreateTime: &timestamppb.Timestamp{Seconds: customCreate}, UpdateTime: &timestamppb.Timestamp{Seconds: customUpdate}}, customCreate, customUpdate, false},
		{"automatic modification", []string{"content", "update_time"}, &apipb.Note{Content: "changed"}, originalCreate, 0, true},
		{"unmasked timestamps ignored", []string{"content"}, &apipb.Note{Content: "changed", CreateTime: &timestamppb.Timestamp{Seconds: customCreate}, UpdateTime: &timestamppb.Timestamp{Seconds: customUpdate}}, originalCreate, originalUpdate, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
				Content: "original", Visibility: apipb.Visibility_PRIVATE,
				CreateTime: &timestamppb.Timestamp{Seconds: originalCreate}, UpdateTime: &timestamppb.Timestamp{Seconds: originalUpdate},
			}})
			require.NoError(t, err)
			tc.patch.Name = note.Name
			beforeSec := time.Now().Unix()
			updated, err := ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
				Note: tc.patch, UpdateMask: &fieldmaskpb.FieldMask{Paths: tc.paths},
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantCreate, updated.CreateTime.Seconds)
			if tc.autoUpdate {
				require.GreaterOrEqual(t, updated.UpdateTime.Seconds, beforeSec)
				require.LessOrEqual(t, updated.UpdateTime.Seconds, time.Now().Unix())
			} else {
				require.Equal(t, tc.wantUpdate, updated.UpdateTime.Seconds)
			}
			// Verify both API reads and raw stored values, not just the update response.
			fetched, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: note.Name})
			require.NoError(t, err)
			require.Equal(t, updated.CreateTime, fetched.CreateTime)
			require.Equal(t, updated.UpdateTime, fetched.UpdateTime)
			id := parseNoteIDFromNameForTest(t, ts, note.Name)
			stored, err := ts.Store.GetNote(ctx, &store.FindNote{ID: &id})
			require.NoError(t, err)
			require.Equal(t, updated.CreateTime.Seconds, stored.CreatedTs)
			require.Equal(t, updated.UpdateTime.Seconds, stored.UpdatedTs)
			wantContent := "original"
			if tc.patch.Content != "" {
				wantContent = tc.patch.Content
			}
			require.Equal(t, wantContent, stored.Content)
		})
	}
}

func TestUpdateNoteRejectsInvalidTimestamps(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	user, err := ts.CreateRegularUser(ctx, "invalid-timestamp-owner")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	note, err := ts.Service.CreateNote(userCtx, &apipb.CreateNoteRequest{Note: &apipb.Note{
		Content: "original", Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	for _, path := range []string{"create_time", "update_time"} {
		for _, timestamp := range []*timestamppb.Timestamp{
			{Seconds: 253402300800},
			{Nanos: -1},
			{Nanos: 1000000000},
			nil,
		} {
			if path == "update_time" && timestamp == nil {
				continue // An omitted update_time explicitly requests the current time.
			}
			t.Run(path+"/"+timestamp.String(), func(t *testing.T) {
				patch := &apipb.Note{Name: note.Name, Content: "must not persist"}
				if path == "create_time" {
					patch.CreateTime = timestamp
				} else {
					patch.UpdateTime = timestamp
				}
				_, err := ts.Service.UpdateNote(userCtx, &apipb.UpdateNoteRequest{
					Note: patch, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content", path}},
				})
				require.Equal(t, codes.InvalidArgument, status.Code(err))
				fetched, err := ts.Service.GetNote(userCtx, &apipb.GetNoteRequest{Name: note.Name})
				require.NoError(t, err)
				require.Equal(t, note.Content, fetched.Content)
				require.Equal(t, note.CreateTime, fetched.CreateTime)
				require.Equal(t, note.UpdateTime, fetched.UpdateTime)
			})
		}
	}
}

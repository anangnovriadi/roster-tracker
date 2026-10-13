package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
)

func TestNoteViewIconLifecycle(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	owner, err := ts.CreateRegularUser(ctx, "icon-owner")
	require.NoError(t, err)
	other, err := ts.CreateRegularUser(ctx, "icon-other")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	parent := "users/" + owner.Username
	emoji := &apipb.NoteView_Icon{Value: &apipb.NoteView_Icon_Emoji{Emoji: "👩🏽‍🌾"}}
	request := &apipb.CreateNoteViewRequest{
		Parent:       parent,
		NoteView:     &apipb.NoteView{Title: "Garden", Filter: "pinned", Icon: emoji},
		ValidateOnly: true,
	}
	validated, err := ts.Service.CreateNoteView(ownerCtx, request)
	require.NoError(t, err)
	require.True(t, proto.Equal(emoji, validated.Icon))
	listed, err := ts.Service.ListNoteViews(ownerCtx, &apipb.ListNoteViewsRequest{Parent: parent})
	require.NoError(t, err)
	require.Empty(t, listed.NoteViews)

	request.ValidateOnly = false
	view, err := ts.Service.CreateNoteView(ownerCtx, request)
	require.NoError(t, err)
	require.True(t, proto.Equal(emoji, view.Icon))
	view, err = ts.Service.UpdateNoteView(ownerCtx, &apipb.UpdateNoteViewRequest{
		NoteView:   &apipb.NoteView{Name: view.Name, Title: "Renamed", Filter: "has_link"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "filter"}},
	})
	require.NoError(t, err)
	require.True(t, proto.Equal(emoji, view.Icon), "other field updates preserve the icon")

	lucide := &apipb.NoteView_Icon{Value: &apipb.NoteView_Icon_Lucide{Lucide: "future-icon"}}
	update := &apipb.UpdateNoteViewRequest{
		NoteView:   &apipb.NoteView{Name: view.Name, Icon: lucide},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"icon"}},
	}
	_, err = ts.Service.UpdateNoteView(ts.CreateUserContext(ctx, other.ID), update)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	view, err = ts.Service.UpdateNoteView(ownerCtx, update)
	require.NoError(t, err)
	require.True(t, proto.Equal(lucide, view.Icon))
	require.Equal(t, "Renamed", view.Title)
	require.Equal(t, "has_link", view.Filter)
	fetched, err := ts.Service.GetNoteView(ownerCtx, &apipb.GetNoteViewRequest{Name: view.Name})
	require.NoError(t, err)
	require.True(t, proto.Equal(lucide, fetched.Icon))
	listed, err = ts.Service.ListNoteViews(ownerCtx, &apipb.ListNoteViewsRequest{Parent: parent})
	require.NoError(t, err)
	require.Len(t, listed.NoteViews, 1)
	require.True(t, proto.Equal(lucide, listed.NoteViews[0].Icon))

	for _, invalid := range []*apipb.NoteView_Icon{
		{},
		{Value: &apipb.NoteView_Icon_Emoji{Emoji: "🌱🌱"}},
		{Value: &apipb.NoteView_Icon_Emoji{Emoji: ""}},
		{Value: &apipb.NoteView_Icon_Lucide{Lucide: "BookOpen"}},
		{Value: &apipb.NoteView_Icon_Lucide{Lucide: ""}},
	} {
		request.NoteView.Icon = invalid
		request.ValidateOnly = true
		_, err = ts.Service.CreateNoteView(ownerCtx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		request.ValidateOnly = false
		_, err = ts.Service.CreateNoteView(ownerCtx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
		update.NoteView.Icon = invalid
		_, err = ts.Service.UpdateNoteView(ownerCtx, update)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	fetched, err = ts.Service.GetNoteView(ownerCtx, &apipb.GetNoteViewRequest{Name: view.Name})
	require.NoError(t, err)
	require.True(t, proto.Equal(lucide, fetched.Icon), "invalid updates leave storage unchanged")

	update.NoteView.Icon = nil
	view, err = ts.Service.UpdateNoteView(ownerCtx, update)
	require.NoError(t, err)
	require.Nil(t, view.Icon)
	fetched, err = ts.Service.GetNoteView(ownerCtx, &apipb.GetNoteViewRequest{Name: view.Name})
	require.NoError(t, err)
	require.Nil(t, fetched.Icon)
}

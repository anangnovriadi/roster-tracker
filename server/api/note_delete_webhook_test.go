package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/internal/webhook"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

func TestDeleteNoteWebhooksRespectCurrentReadAccess(t *testing.T) {
	ctx := context.Background()
	service := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, service, "delete-webhook-owner", store.RoleUser)
	successor := createSpaceTestUser(ctx, t, service, "delete-webhook-successor", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	successorCtx := userCtx(ctx, successor.ID)

	space, err := service.CreateSpace(ownerCtx, &apipb.CreateSpaceRequest{
		SpaceId: "delete-webhook-space",
		Space:   &apipb.Space{Title: "Delete webhook space"},
	})
	require.NoError(t, err)
	inviteAndAcceptSpaceTestUser(ctx, t, service, owner, successor, space, apipb.SpaceMember_ADMIN)

	assignedSpace := space.Name
	hiddenNote, err := service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "must not leave through a webhook",
			Visibility: apipb.Visibility_SPACE,
			Space:      &assignedSpace,
		},
	})
	require.NoError(t, err)
	readableNote, err := service.CreateNote(ownerCtx, &apipb.CreateNoteRequest{
		Note: &apipb.Note{
			Content:    "readable deletion payload",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	_, err = service.DeleteSpaceMember(successorCtx, &apipb.DeleteSpaceMemberRequest{
		Name: buildSpaceMemberName("delete-webhook-space", owner.Username),
	})
	require.NoError(t, err)
	_, err = service.GetNote(ownerCtx, &apipb.GetNoteRequest{Name: hiddenNote.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	received := make(chan []byte, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer receiver.Close()

	receiverHost, _, err := net.SplitHostPort(receiver.Listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, webhook.ConfigurePrivateDestinationAllowlist([]string{receiverHost}))
	defer func() {
		require.NoError(t, webhook.ConfigurePrivateDestinationAllowlist(nil))
	}()
	_, err = service.CreateUserWebhook(ownerCtx, &apipb.CreateUserWebhookRequest{
		Parent: BuildUserName(owner.Username),
		Webhook: &apipb.UserWebhook{
			DisplayName: "delete-capture",
			Url:         receiver.URL,
		},
	})
	require.NoError(t, err)

	_, err = service.DeleteNote(ownerCtx, &apipb.DeleteNoteRequest{Name: hiddenNote.Name})
	require.NoError(t, err, "the author retains narrow lifecycle authority after losing membership")
	select {
	case body := <-received:
		require.Failf(t, "unreadable note leaked through webhook", "received %s", body)
	case <-time.After(500 * time.Millisecond):
	}

	_, err = service.DeleteNote(ownerCtx, &apipb.DeleteNoteRequest{Name: readableNote.Name})
	require.NoError(t, err)
	var body []byte
	select {
	case body = <-received:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "readable note deletion webhook was not delivered")
	}
	payload := &noteWebhookPayload{}
	require.NoError(t, json.Unmarshal(body, payload))
	require.Equal(t, "notes.note.deleted", payload.ActivityType)
	require.NotNil(t, payload.Note)
	require.Equal(t, readableNote.Name, payload.Note.Name)
	require.Equal(t, "readable deletion payload", payload.Note.Content)
	require.NotContains(t, string(body), hiddenNote.Content)

	select {
	case extra := <-received:
		require.Failf(t, "unexpected additional deletion webhook", "received %s", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

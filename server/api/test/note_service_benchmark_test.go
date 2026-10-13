package test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/anangnovriadi/roster-tracker/internal/profile"
	"github.com/anangnovriadi/roster-tracker/internal/version"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/server/api"
	"github.com/anangnovriadi/roster-tracker/server/auth"
	"github.com/anangnovriadi/roster-tracker/store"
	"github.com/anangnovriadi/roster-tracker/store/db"
)

const (
	benchmarkTopLevelNoteCount = 5000
	benchmarkPageSize          = 16
)

type benchmarkService struct {
	*TestService
	hostUser          *store.User
	authenticatedCtx  context.Context
	publicCtx         context.Context
	pageTenToken      string
	commentParentName string
}

func newBenchmarkService(tb testing.TB) *benchmarkService {
	tb.Helper()

	ctx := context.Background()
	testService := newTestingServiceForTB(tb)

	hostUser, err := testService.CreateHostUser(ctx, "bench-host")
	if err != nil {
		tb.Fatalf("failed to create host user: %v", err)
	}

	commentParentName, err := seedListNotesBenchmarkData(ctx, testService.Store, hostUser)
	if err != nil {
		tb.Fatalf("failed to seed benchmark data: %v", err)
	}

	authenticatedCtx := context.WithValue(context.Background(), auth.UserIDContextKey, hostUser.ID)
	pageTenToken, err := getListNotesPageToken(authenticatedCtx, testService.Service, 10, benchmarkPageSize)
	if err != nil {
		tb.Fatalf("failed to build page token: %v", err)
	}

	return &benchmarkService{
		TestService:       testService,
		hostUser:          hostUser,
		authenticatedCtx:  authenticatedCtx,
		publicCtx:         context.Background(),
		pageTenToken:      pageTenToken,
		commentParentName: commentParentName,
	}
}

func newTestingServiceForTB(tb testing.TB) *TestService {
	tb.Helper()

	ctx := context.Background()
	dataDir := tb.TempDir()
	testProfile := getBenchmarkProfile(dataDir)
	dbDriver, err := db.NewDBDriver(testProfile)
	if err != nil {
		tb.Fatalf("failed to create db driver: %v", err)
	}

	testStore := store.New(dbDriver, testProfile)
	if err := testStore.Migrate(ctx); err != nil {
		tb.Fatalf("failed to migrate db: %v", err)
	}
	tb.Cleanup(func() {
		testStore.Close()
	})

	service := newServiceWithProfile(testProfile, testStore)
	return &TestService{
		Service: service,
		Store:   testStore,
		Profile: testProfile,
		Secret:  service.Secret,
	}
}

func getBenchmarkProfile(dataDir string) *profile.Profile {
	return &profile.Profile{
		Demo:        true,
		Version:     version.GetCurrentVersion(),
		InstanceURL: "http://localhost:8080",
		Driver:      "sqlite",
		DSN:         filepath.Join(dataDir, "bench.db"),
		Data:        dataDir,
	}
}

func newServiceWithProfile(testProfile *profile.Profile, testStore *store.Store) *api.APIService {
	service := api.NewAPIService("bench-secret", testProfile, testStore)
	return service
}

func seedListNotesBenchmarkData(ctx context.Context, stores *store.Store, hostUser *store.User) (string, error) {
	topLevelNotes := make([]*store.Note, 0, benchmarkTopLevelNoteCount)
	commentParentName := ""

	for i := range benchmarkTopLevelNoteCount {
		visibility := store.Private
		if i%4 == 0 {
			visibility = store.Public
		}
		note, err := stores.CreateNote(ctx, &store.Note{
			UID:        fmt.Sprintf("note-%06d", i),
			CreatorID:  hostUser.ID,
			Content:    benchmarkNoteContent(i),
			Visibility: visibility,
		})
		if err != nil {
			return "", err
		}
		topLevelNotes = append(topLevelNotes, note)

		if i%3 == 0 {
			if _, err := stores.CreateAttachment(ctx, &store.Attachment{
				UID:       fmt.Sprintf("att-%06d", i),
				CreatorID: hostUser.ID,
				Filename:  fmt.Sprintf("note-%06d.png", i),
				Type:      "image/png",
				Size:      2048,
				NoteID:    &note.ID,
			}); err != nil {
				return "", err
			}
		}

		if i%5 == 0 {
			if _, err := stores.UpsertReaction(ctx, &store.Reaction{
				CreatorID:    hostUser.ID,
				NoteID:       note.ID,
				ReactionType: "thumbs-up",
			}); err != nil {
				return "", err
			}
		}
	}

	for i, note := range topLevelNotes {
		if i+1 < len(topLevelNotes) && i%4 == 0 {
			if _, err := stores.UpsertNoteRelation(ctx, &store.NoteRelation{
				NoteID:        note.ID,
				RelatedNoteID: topLevelNotes[i+1].ID,
				Type:          store.NoteRelationReference,
			}); err != nil {
				return "", err
			}
		}

		if i%6 == 0 {
			_, err := stores.CreateNoteComment(ctx, &store.Note{
				UID:        fmt.Sprintf("comment-%06d", i),
				CreatorID:  hostUser.ID,
				Content:    fmt.Sprintf("Comment for note %06d", i),
				Visibility: store.Private,
			}, note.ID, hostUser.ID)
			if err != nil {
				return "", err
			}
			if commentParentName == "" {
				commentParentName = "notes/" + note.UID
			}
		}
	}

	return commentParentName, nil
}

func benchmarkNoteContent(i int) string {
	return fmt.Sprintf("# Bench Note %06d\n\nThis is benchmark note %06d with enough content to exercise snippet generation.\n\n- task one\n- task two\n", i, i)
}

func getListNotesPageToken(ctx context.Context, service *api.APIService, page int, pageSize int32) (string, error) {
	pageToken := ""
	for range page - 1 {
		resp, err := service.ListNotes(ctx, &apipb.ListNotesRequest{
			PageSize:  pageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return "", err
		}
		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return pageToken, nil
}

func BenchmarkListNotes(b *testing.B) {
	bench := newBenchmarkService(b)

	b.Run("authenticated_first_page", func(b *testing.B) {
		req := &apipb.ListNotesRequest{PageSize: benchmarkPageSize}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := bench.Service.ListNotes(bench.authenticatedCtx, req)
			if err != nil {
				b.Fatalf("ListNotes failed: %v", err)
			}
			if len(resp.Notes) == 0 {
				b.Fatal("expected notes in authenticated benchmark response")
			}
		}
	})

	b.Run("authenticated_page_ten", func(b *testing.B) {
		req := &apipb.ListNotesRequest{PageSize: benchmarkPageSize, PageToken: bench.pageTenToken}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := bench.Service.ListNotes(bench.authenticatedCtx, req)
			if err != nil {
				b.Fatalf("ListNotes failed: %v", err)
			}
			if len(resp.Notes) == 0 {
				b.Fatal("expected notes in paged benchmark response")
			}
		}
	})

	b.Run("public_first_page", func(b *testing.B) {
		req := &apipb.ListNotesRequest{PageSize: benchmarkPageSize}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := bench.Service.ListNotes(bench.publicCtx, req)
			if err != nil {
				b.Fatalf("ListNotes failed: %v", err)
			}
			if len(resp.Notes) == 0 {
				b.Fatal("expected notes in public benchmark response")
			}
		}
	})
}

func BenchmarkListNoteCommentsPreview(b *testing.B) {
	bench := newBenchmarkService(b)
	if bench.commentParentName == "" {
		b.Fatal("expected seeded note with comments")
	}

	req := &apipb.ListNoteCommentsRequest{
		Parent:   bench.commentParentName,
		PageSize: 3,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := bench.Service.ListNoteComments(bench.authenticatedCtx, req)
		if err != nil {
			b.Fatalf("ListNoteComments failed: %v", err)
		}
		if len(resp.Comments) == 0 {
			b.Fatal("expected comments in benchmark response")
		}
	}
}

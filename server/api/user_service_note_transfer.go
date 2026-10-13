package api

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"time"

	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/anangnovriadi/roster-tracker/core/noteexport"
	"github.com/anangnovriadi/roster-tracker/internal/ratelimit"
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	"github.com/anangnovriadi/roster-tracker/store"
)

const (
	importNotesProcedure = "/roster.api.UserService/ImportNotes"
	noteImportTempPrefix = ".notes-rpc-import-"
	// maxNoteImportBytes caps one staged archive.
	maxNoteImportBytes = 4 << 30
)

// noteImportState is what a staged archive carries besides its bytes: the
// report, once the finishing call has imported it.
type noteImportState struct {
	report *apipb.ImportNotesReport
}

type noteImports = uploadSessions[noteImportState]

// resolveNoteTransferOwner resolves users/{user} and asserts it is the caller.
func (s *APIService) resolveNoteTransferOwner(ctx context.Context, name string) (*store.User, error) {
	user, err := ResolveUserByName(ctx, s.Store, name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	if err := s.requireCallerIs(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// ExportNotes writes the caller's Roster Tracker export file into the response body.
func (s *APIService) ExportNotes(ctx context.Context, request *apipb.ExportNotesRequest) (*httpbody.HttpBody, error) {
	user, err := s.resolveNoteTransferOwner(ctx, request.Name)
	if err != nil {
		return nil, err
	}
	if err := s.throttleAndCharge(ratelimit.ScopeArchiveUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}
	var archive bytes.Buffer
	if err := s.WriteNoteExport(ctx, user, &archive); err != nil {
		slog.Error("note export failed", slog.Int("user", int(user.ID)), slog.Any("error", err))
		return nil, status.Error(codes.Internal, "failed to export notes")
	}
	return &httpbody.HttpBody{ContentType: noteexport.MediaType, Data: archive.Bytes()}, nil
}

// ImportNotes stages an archive in chunks and, on the finishing call, plans
// or imports it. See the proto comment for the protocol.
func (s *APIService) ImportNotes(ctx context.Context, request *apipb.ImportNotesRequest) (*apipb.ImportNotesResponse, error) {
	user, err := s.resolveNoteTransferOwner(ctx, request.Name)
	if err != nil {
		return nil, err
	}
	// Importing creates notes, which only members may do.
	if user.Role == store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "admins cannot create notes")
	}
	if len(request.Data) > uploadChunkSize {
		return nil, status.Errorf(codes.ResourceExhausted, "upload chunk exceeds the limit")
	}
	var id string
	var upload *uploadSession[noteImportState]
	switch u := request.Upload.(type) {
	case *apipb.ImportNotesRequest_Spec:
		if err := s.throttleAndCharge(ratelimit.ScopeArchiveUser, userKey(user.ID), 1); err != nil {
			return nil, err
		}
		if u.Spec.TotalSize < 0 || u.Spec.TotalSize > maxNoteImportBytes {
			return nil, status.Errorf(codes.InvalidArgument, "invalid total_size")
		}
		if request.WriteOffset != 0 {
			return nil, status.Errorf(codes.OutOfRange, "write_offset must be 0 for a new upload")
		}
		if int64(len(request.Data)) > u.Spec.TotalSize {
			return nil, status.Errorf(codes.InvalidArgument, "data exceeds total_size")
		}
		id, upload, err = s.noteImports.create(s.Profile.Data, noteImportTempPrefix, user.ID, u.Spec.TotalSize, noteImportState{})
		if err != nil {
			return nil, err
		}
		upload.mu.Lock()
	case *apipb.ImportNotesRequest_UploadId:
		id = u.UploadId
		upload, err = s.noteImports.resume(id, user.ID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "spec or upload_id is required")
	}
	defer upload.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if err := upload.write(request.WriteOffset, request.Data, request.FinishWrite); err != nil {
		return nil, err
	}
	response := &apipb.ImportNotesResponse{UploadId: id, CommittedSize: upload.committedSize, MaxChunkSize: uploadChunkSize}
	if upload.complete {
		response.Result = &apipb.ImportNotesResponse_Report{Report: upload.state.report}
	} else if request.FinishWrite {
		if upload.committedSize != upload.totalSize {
			return nil, status.Errorf(codes.FailedPrecondition, "upload is incomplete")
		}
		if err := s.finishNoteImport(ctx, user, upload, request, response); err != nil {
			return nil, err
		}
	}
	upload.expireTime = time.Now().Add(uploadTTL)
	return response, nil
}

// finishNoteImport reads the staged archive and either plans or imports it,
// setting the response result. An import completes the session; a plan
// leaves it staged.
func (s *APIService) finishNoteImport(ctx context.Context, user *store.User, upload *uploadSession[noteImportState], request *apipb.ImportNotesRequest, response *apipb.ImportNotesResponse) error {
	file, err := os.Open(upload.path)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to open upload file: %v", err)
	}
	defer file.Close()
	archive, err := noteexport.Read(file, upload.totalSize)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid note export file: %v", err)
	}
	if request.ValidateOnly {
		// A plan can be requested again and again on the same staged archive,
		// each time re-parsing it in full, so it costs an archive unit too.
		if err := s.throttleAndCharge(ratelimit.ScopeArchiveUser, userKey(user.ID), 1); err != nil {
			return err
		}
		plan, err := s.PlanNoteImport(ctx, user, archive)
		if err != nil {
			slog.Error("note import planning failed", slog.Int("user", int(user.ID)), slog.Any("error", err))
			return status.Error(codes.Internal, "failed to inspect the archive")
		}
		response.Result = &apipb.ImportNotesResponse_Plan{Plan: plan}
		return nil
	}
	report, err := s.ImportNoteExport(ctx, user, archive, request.ConflictPolicy)
	if err != nil {
		slog.Error("note import failed", slog.Int("user", int(user.ID)), slog.Any("error", err))
		return status.Error(codes.Internal, "failed to import notes")
	}
	upload.state.report = report
	upload.complete = true
	os.Remove(upload.path)
	response.Result = &apipb.ImportNotesResponse_Report{Report: report}
	return nil
}

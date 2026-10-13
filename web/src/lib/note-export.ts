import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { userServiceClient } from "@/connect";
import { downloadFileFromUrl } from "@/lib/browser";
import type { ImportNotesPlan, ImportNotesReport, ImportNotesRequest } from "@/types/proto/api/user_service_pb";
import { ImportNotesRequest_ConflictPolicy, ImportNotesSpecSchema } from "@/types/proto/api/user_service_pb";

const DEFAULT_CHUNK_SIZE = 2 * 1024 * 1024;

// exportNotes downloads the user's Roster Tracker export file.
export async function exportNotes(userName: string, username: string): Promise<void> {
  const body = await userServiceClient.exportNotes({ name: userName });
  const stamp = new Date().toISOString().replace(/[-:]/g, "").replace(/\.\d+/, "");
  const url = URL.createObjectURL(new Blob([body.data as BlobPart], { type: body.contentType }));
  downloadFileFromUrl(url, `notes-export-${username}-${stamp}.zip`);
  // Give the browser a tick to start the download before revoking.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export interface StagedNoteImport {
  uploadId: string;
  size: number;
  plan: ImportNotesPlan;
}

async function sendChunk(request: ImportNotesRequest, signal?: AbortSignal) {
  // Retrying the identical last write is safe even if its response was lost.
  for (let attempt = 0; ; attempt++) {
    try {
      return await userServiceClient.importNotes(request, { signal });
    } catch (error) {
      if (signal?.aborted || ConnectError.from(error).code !== Code.Unavailable || attempt >= 2) throw error;
    }
  }
}

// stageNoteImport uploads the archive in chunks and finishes with
// validate_only, returning the staged upload and the server's plan.
export async function stageNoteImport(userName: string, file: File, signal?: AbortSignal): Promise<StagedNoteImport> {
  const spec = create(ImportNotesSpecSchema, { totalSize: BigInt(file.size) });
  const initial = await userServiceClient.importNotes({ name: userName, upload: { case: "spec", value: spec } }, { signal });
  if (!initial.uploadId || initial.committedSize !== 0n || initial.maxChunkSize <= 0) {
    throw new Error("Invalid upload initialization response");
  }
  const chunkSize = Math.min(initial.maxChunkSize, DEFAULT_CHUNK_SIZE);
  for (let offset = 0; ; ) {
    signal?.throwIfAborted();
    const end = Math.min(offset + chunkSize, file.size);
    const response = await sendChunk(
      {
        name: userName,
        upload: { case: "uploadId", value: initial.uploadId },
        writeOffset: BigInt(offset),
        data: new Uint8Array(await file.slice(offset, end).arrayBuffer()),
        finishWrite: end === file.size,
        validateOnly: true,
      } as ImportNotesRequest,
      signal,
    );
    if (response.committedSize !== BigInt(end)) {
      throw new Error("Unexpected upload offset");
    }
    if (end === file.size) {
      if (response.result.case !== "plan") throw new Error("Upload finished without a plan");
      return { uploadId: initial.uploadId, size: file.size, plan: response.result.value };
    }
    offset = end;
  }
}

// importStagedNotes imports a staged archive with the chosen policy.
export async function importStagedNotes(
  userName: string,
  staged: StagedNoteImport,
  conflictPolicy: ImportNotesRequest_ConflictPolicy,
): Promise<ImportNotesReport> {
  const response = await userServiceClient.importNotes({
    name: userName,
    upload: { case: "uploadId", value: staged.uploadId },
    writeOffset: BigInt(staged.size),
    finishWrite: true,
    conflictPolicy,
  });
  if (response.result.case !== "report") throw new Error("Import finished without a report");
  return response.result.value;
}

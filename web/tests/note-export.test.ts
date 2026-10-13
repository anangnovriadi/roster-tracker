import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { exportNotes, stageNoteImport } from "@/lib/note-export";

const { exportRequest, importRequest, download } = vi.hoisted(() => ({
  exportRequest: vi.fn(),
  importRequest: vi.fn(),
  download: vi.fn(),
}));

vi.mock("@/connect", () => ({ userServiceClient: { exportNotes: exportRequest, importNotes: importRequest } }));
vi.mock("@/lib/browser", () => ({ downloadFileFromUrl: download }));

describe("note export files", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("downloads a ZIP with the export name and releases its object URL", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-20T08:30:00Z"));
    const createURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:export");
    const revokeURL = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    exportRequest.mockResolvedValue({ contentType: "application/vnd.roster-tracker.export+zip", data: new Uint8Array([80, 75]) });

    await exportNotes("users/alice", "alice");

    expect(exportRequest).toHaveBeenCalledWith({ name: "users/alice" });
    expect(createURL.mock.calls[0][0]).toMatchObject({ type: "application/vnd.roster-tracker.export+zip", size: 2 });
    expect(download).toHaveBeenCalledWith("blob:export", "notes-export-alice-20260920T083000Z.zip");
    expect(revokeURL).not.toHaveBeenCalled();
    await vi.runAllTimersAsync();
    expect(revokeURL).toHaveBeenCalledWith("blob:export");
  });

  it.each([
    ["golden.zip", "notes-export-alice.zip", "application/vnd.roster-tracker.export+zip"],
    ["legacy.zip", "notes-archive-alice.zip", "application/vnd.roster-tracker.archive+zip"],
    ["legacy.zip", "my-notes.zip", "application/zip"],
  ])("uploads %s as %s without filtering by the filename or MIME type", async (fixture, name, type) => {
    const bytes = readFileSync(resolve(import.meta.dirname, "../../core/noteexport/testdata/1.0", fixture));
    const file = new File([bytes], name, { type });
    const plan = { noteCount: 3 };
    importRequest.mockImplementation(async (request) => ({
      uploadId: "upload-id",
      maxChunkSize: 1024,
      committedSize: request.data ? request.writeOffset + BigInt(request.data.length) : 0n,
      result: request.finishWrite ? { case: "plan", value: plan } : { case: undefined },
    }));

    await expect(stageNoteImport("users/alice", file)).resolves.toEqual({ uploadId: "upload-id", size: bytes.length, plan });

    const chunks = importRequest.mock.calls.slice(1).map(([request]) => request.data);
    expect(Buffer.concat(chunks)).toEqual(bytes);
    expect(importRequest.mock.lastCall?.[0]).toMatchObject({ finishWrite: true, validateOnly: true });
  });
});

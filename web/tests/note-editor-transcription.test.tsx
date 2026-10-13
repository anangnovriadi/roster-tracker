import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import NoteEditor from "@/components/NoteEditor";
import { transcriptionService } from "@/components/NoteEditor/services";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { NoteSchema, Visibility } from "@/types/proto/api/note_service_pb";

vi.mock("@/hooks/useUserQueries", () => ({ useTagCounts: () => ({ data: {} }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/1" }) }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ userGeneralSetting: {} }) }));
vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({
    aiSetting: { providers: [{ id: "configured", apiKeySet: true }], transcription: { providerId: "configured" } },
    fetchSetting: vi.fn().mockResolvedValue(undefined),
  }),
}));
vi.mock("@/contexts/NewNoteContext", () => ({ useNewNote: () => ({ markNewNote: vi.fn() }) }));
const clients = vi.hoisted(() => ({ getNote: vi.fn(), updateNote: vi.fn(), listNoteAttachments: vi.fn() }));
vi.mock("@/connect", () => ({ attachmentServiceClient: {}, noteServiceClient: clients, userServiceClient: {} }));
vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

describe("existing note audio transcription", () => {
  it("blocks save while transcribing, inserts into the editable draft and retains the original attachment on save", async () => {
    const attachment = create(AttachmentSchema, { name: "attachments/voice", filename: "voice.wav", type: "audio/wav" });
    const note = create(NoteSchema, {
      name: "notes/meeting",
      creator: "users/1",
      content: "Original meeting note",
      visibility: Visibility.PRIVATE,
      attachments: [attachment],
    });
    clients.getNote.mockResolvedValue(note);
    clients.updateNote.mockImplementation(async ({ note: update }) => create(NoteSchema, { ...note, ...update }));
    const pending = Promise.withResolvers<string>();
    vi.spyOn(transcriptionService, "transcribeAttachment").mockReturnValue(pending.promise);
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <NoteEditor note={note} />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(container.querySelector(".cm-content")?.textContent).toBe(note.content));
    fireEvent.click(screen.getByRole("button", { name: "Transcribe" }));
    expect(screen.getByRole("button", { name: "Update" })).toBeDisabled();
    await act(async () => {
      pending.resolve("Recognized meeting discussion");
      await pending.promise;
    });
    await waitFor(() => expect(container.querySelector(".cm-content")?.textContent).toContain("Recognized meeting discussion"));
    expect(container.querySelector(".cm-content")?.textContent).toContain(note.content);
    expect(screen.getByText("voice.wav")).toBeInTheDocument();
    expect(clients.updateNote).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => expect(clients.updateNote).toHaveBeenCalledOnce());
    const request = clients.updateNote.mock.calls[0][0];
    expect(request.note.content).toContain("Recognized meeting discussion");
    expect(request.note.content).toContain(note.content);
    expect(request.updateMask.paths).not.toContain("attachments");
  });
});

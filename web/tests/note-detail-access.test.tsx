import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { noteKeys } from "@/hooks/useNoteQueries";
import NoteDetail from "@/pages/NoteDetail";
import { type Note, NoteSchema } from "@/types/proto/api/note_service_pb";

const state = vi.hoisted(() => ({
  currentUser: undefined as { name: string } | undefined,
  getNote: vi.fn(),
  listNoteComments: vi.fn(),
  setNoteDetail: vi.fn(),
  toastError: vi.fn(),
}));
vi.mock("@/connect", () => ({ noteServiceClient: state }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ currentUser: state.currentUser, isInitialized: true }) }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ isInitialized: true }) }));
vi.mock("@/contexts/AppSidebarContext", () => ({ useAppSidebar: () => ({ setNoteDetail: state.setNoteDetail }) }));
vi.mock("@/components/NoteView", () => ({ default: ({ note }: { note: Note }) => <article>{note.content}</article> }));
vi.mock("@/components/NoteView/NoteViewContext", () => ({ computeCommentAmount: () => 0 }));
vi.mock("@/components/NoteCommentSection", () => ({ default: () => null }));
vi.mock("@/components/NoteContent/MentionResolutionContext", () => ({
  MentionResolutionProvider: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("react-hot-toast", () => ({ toast: { error: state.toastError } }));

function renderDetail(client: QueryClient, name = "notes/restored") {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/${name}`]}>
        <Routes>
          <Route path="/notes/:uid" element={<NoteDetail />} />
          <Route path="/404" element={<div>not found</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
const createClient = () => new QueryClient({ defaultOptions: { queries: { retry: false, retryDelay: 0 } } });

describe("note detail access recovery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.currentUser = undefined;
    state.listNoteComments.mockResolvedValue({ comments: [], nextPageToken: "" });
  });

  it.each(["fresh", "stale"])("revalidates a %s cached denial before showing a restored note", async (freshness) => {
    const client = createClient();
    const name = "notes/restored";
    client.setQueryData(noteKeys.detail(name), null, { updatedAt: freshness === "fresh" ? Date.now() : 1 });
    let resolve!: (note: Note) => void;
    let signal!: AbortSignal;
    state.getNote.mockImplementation((_, options) => {
      signal = options.signal;
      return new Promise<Note>((done) => {
        resolve = done;
      });
    });
    renderDetail(client);
    await waitFor(() => expect(state.getNote).toHaveBeenCalledOnce());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    expect(signal.aborted).toBe(false);
    await act(async () => resolve(create(NoteSchema, { name, content: "Restored note" })));
    expect(await screen.findByText("Restored note")).toBeInTheDocument();
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    client.clear();
  });

  it.each([Code.PermissionDenied, Code.NotFound])("redirects only after revalidation confirms denial (%s)", async (code) => {
    const client = createClient();
    client.setQueryData(noteKeys.detail("notes/restored"), null, { updatedAt: 1 });
    let reject!: (error: Error) => void;
    state.getNote.mockImplementation(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        }),
    );
    renderDetail(client);
    await waitFor(() => expect(state.getNote).toHaveBeenCalledOnce());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    await act(async () => reject(new ConnectError("denied", code)));
    expect(await screen.findByText("not found")).toBeInTheDocument();
    client.clear();
  });

  it("does not treat a failed revalidation as a renewed denial", async () => {
    const client = createClient();
    client.setQueryData(noteKeys.detail("notes/restored"), null, { updatedAt: 1 });
    state.getNote.mockRejectedValue(new ConnectError("offline", Code.Unavailable));
    renderDetail(client);
    await waitFor(() => expect(state.toastError).toHaveBeenCalled());
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    client.clear();
  });

  it.each([false, true])("classifies parent 401 according to the current viewer (signed in: %s)", async (signedIn) => {
    if (signedIn) state.currentUser = { name: "users/alice" };
    const client = createClient();
    const comment = create(NoteSchema, { name: "notes/comment", content: "Public comment", parent: "notes/private-parent" });
    state.getNote.mockImplementation(({ name }) =>
      name === comment.name ? Promise.resolve(comment) : Promise.reject(new ConnectError("authentication required", Code.Unauthenticated)),
    );
    renderDetail(client, comment.name);
    expect(await screen.findByText(signedIn ? "note.parent-load-error" : "note.parent-unavailable")).toBeInTheDocument();
    expect(screen.getByText("Public comment")).toBeInTheDocument();
    expect(screen.queryByText("not found")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    if (signedIn) expect(screen.getByRole("button", { name: "search.retry" })).toBeInTheDocument();
    else expect(screen.queryByRole("button", { name: "search.retry" })).not.toBeInTheDocument();
    expect(state.setNoteDetail).toHaveBeenLastCalledWith(
      expect.objectContaining({
        parentStatus: signedIn ? "error" : "unavailable",
        parentNote: undefined,
      }),
    );
    client.clear();
  });

  it("lets a guest retry a transient parent error", async () => {
    const client = createClient();
    const comment = create(NoteSchema, { name: "notes/comment", content: "Public comment", parent: "notes/parent" });
    let parentAvailable = false;
    state.getNote.mockImplementation(({ name }) =>
      name === comment.name
        ? Promise.resolve(comment)
        : parentAvailable
          ? Promise.resolve(create(NoteSchema, { name, content: "Parent note" }))
          : Promise.reject(new ConnectError("offline", Code.Unavailable)),
    );
    renderDetail(client, comment.name);
    const retry = await screen.findByRole("button", { name: "search.retry" });
    parentAvailable = true;
    fireEvent.click(retry);
    expect(await screen.findByRole("link", { name: "Parent note" })).toBeInTheDocument();
    expect(screen.getByText("Public comment")).toBeInTheDocument();
    client.clear();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNoteActionHandlers } from "@/components/NoteActionMenu/hooks";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";

const mocks = vi.hoisted(() => ({
  updateNote: vi.fn(),
  deleteNote: vi.fn(),
  copy: vi.fn(),
}));

vi.mock("copy-to-clipboard", () => ({ default: mocks.copy }));

vi.mock("@/hooks/useNoteQueries", () => ({
  noteKeys: {
    comments: (name: string) => ["notes", name, "comments"],
    detail: (name: string) => ["notes", name, "detail"],
  },
  useUpdateNote: () => ({ mutateAsync: mocks.updateNote }),
  useDeleteNote: () => ({ mutateAsync: mocks.deleteNote }),
}));

vi.mock("@/hooks/useUserQueries", () => ({
  userKeys: { stats: () => ["users", "stats"] },
}));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ profile: { instanceUrl: "" } }),
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

vi.mock("react-hot-toast", () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const createNote = (state: State, parent = ""): Note =>
  ({
    name: "notes/1",
    creator: "users/alice",
    content: "note",
    state,
    parent,
  }) as Note;

const renderActions = (state: State, parent = "", parentPage?: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/notes/1"]}>{children}</MemoryRouter>
    </QueryClientProvider>
  );

  const rendered = renderHook(
    () => {
      const location = useLocation();
      const handlers = useNoteActionHandlers({
        note: createNote(state, parent),
        parentPage,
        setDeleteDialogOpen: vi.fn(),
      });
      return { handlers, pathname: location.pathname, search: location.search };
    },
    { wrapper },
  );
  return { ...rendered, invalidateQueries };
};

describe("Note detail mutation navigation", () => {
  beforeEach(() => {
    mocks.updateNote.mockReset().mockResolvedValue(undefined);
    mocks.deleteNote.mockReset().mockResolvedValue(undefined);
    mocks.copy.mockReset();
  });

  it.each([
    [State.NORMAL, "/"],
    [State.ARCHIVED, "/"],
  ])("switches a %s note to its user-level destination", async (state, destination) => {
    const { result } = renderActions(state);

    await act(async () => {
      await result.current.handlers.handleToggleNoteStatusClick();
    });

    await waitFor(() => expect(result.current.pathname).toBe(destination));
  });

  it("returns to the full Space origin after deleting a note detail", async () => {
    const origin = "/spaces/product/calendar/2026/09/06?filter=tagSearch%3Awork";
    const { result } = renderActions(State.NORMAL, "", origin);
    await act(() => result.current.handlers.confirmDeleteNote());
    expect(result.current.pathname + result.current.search).toBe(origin);
  });

  it("returns directly to a global origin after deleting", async () => {
    const { result } = renderActions(State.NORMAL);

    await act(async () => {
      await result.current.handlers.confirmDeleteNote();
    });

    await waitFor(() => expect(result.current.pathname).toBe("/"));
  });

  it("refreshes a parent note after deleting one of its comments", async () => {
    const { result, invalidateQueries } = renderActions(State.NORMAL, "notes/parent");

    await act(async () => {
      await result.current.handlers.confirmDeleteNote();
    });

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["notes", "notes/parent", "comments"] });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["notes", "notes/parent", "detail"] });
  });

  it.each([
    ["", "http://localhost:3000/notes/1"],
    ["notes/parent", "http://localhost:3000/notes/parent#1"],
  ])("copies a link that lands on the note, or on the comment inside its parent (parent: %s)", (parent, link) => {
    const { result } = renderActions(State.NORMAL, parent);
    act(() => result.current.handlers.handleCopyLink());
    expect(mocks.copy).toHaveBeenCalledWith(link);
  });
});

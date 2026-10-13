import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useResolvedRelationNotes } from "@/components/NoteMetadata/Relation/useResolvedRelationNotes";
import { findNoteInCollectionQueries, noteKeys, useMemo, useUpdateNote } from "@/hooks/useNoteQueries";
import { ListNotesResponseSchema, NoteRelation_Type, NoteRelationSchema, NoteSchema } from "@/types/proto/api/note_service_pb";

const api = vi.hoisted(() => ({ getNote: vi.fn(), updateNote: vi.fn() }));
vi.mock("@/connect", () => ({ noteServiceClient: api }));
const setup = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryDelay: 0 }, mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  return { client, wrapper };
};

describe("note access transitions", () => {
  beforeEach(() => vi.resetAllMocks());

  it.each([
    Code.PermissionDenied,
    Code.NotFound,
  ])("discards denied parent content and snippets while preserving the comment (%s)", async (code) => {
    const { client, wrapper } = setup();
    const parent = create(NoteSchema, { name: "notes/parent", content: "secret", snippet: "secret" });
    const comment = create(NoteSchema, {
      name: "notes/comment",
      parent: parent.name,
      content: "my comment",
      relations: [
        create(NoteRelationSchema, {
          type: NoteRelation_Type.COMMENT,
          note: { name: "notes/comment" },
          relatedNote: { name: parent.name, snippet: "secret" },
        }),
      ],
    });
    client.setQueryData(noteKeys.detail(parent.name), parent, { updatedAt: 1 });
    client.setQueryData(noteKeys.detail(comment.name), comment);
    client.setQueryData(noteKeys.list({}), create(ListNotesResponseSchema, { notes: [parent, comment] }));
    api.getNote.mockRejectedValue(new ConnectError("denied", code));
    const { result } = renderHook(() => useMemo(parent.name), { wrapper });
    await waitFor(() => expect(result.current.isUnavailable).toBe(true));
    expect(result.current.data).toBeUndefined();
    expect(client.getQueryData(noteKeys.detail(parent.name))).toBeNull();
    expect(findNoteInCollectionQueries(client, parent.name)).toBeUndefined();
    expect(client.getQueryData(noteKeys.detail(comment.name))).toMatchObject({ content: "my comment", parent: parent.name, relations: [] });
    expect(api.getNote).toHaveBeenCalledOnce();
    api.getNote.mockResolvedValue(parent);
    await act(async () => {
      await result.current.refetch();
    });
    await waitFor(() => expect(result.current.isUnavailable).toBe(false));
    expect(result.current.data?.content).toBe("secret");
    client.clear();
  });

  it("resolves readable relations even when another relation is denied", async () => {
    const { client, wrapper } = setup();
    api.getNote.mockImplementation(({ name }) =>
      name === "notes/denied"
        ? Promise.reject(new ConnectError("denied", Code.PermissionDenied))
        : Promise.resolve(create(NoteSchema, { name, snippet: "Readable" })),
    );
    const { result } = renderHook(() => useResolvedRelationNotes(["notes/denied", "notes/readable"]), { wrapper });
    await waitFor(() => expect(result.current["notes/readable"]?.snippet).toBe("Readable"));
    expect(result.current["notes/denied"]).toBeNull();
    client.clear();
  });

  it("retains transient errors as retryable errors instead of marking a note unavailable", async () => {
    const { client, wrapper } = setup();
    api.getNote.mockRejectedValue(new ConnectError("offline", Code.Unavailable));
    const { result } = renderHook(() => useMemo("notes/parent"), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.isUnavailable).toBe(false);
    expect(api.getNote).toHaveBeenCalledTimes(2);
    client.clear();
  });

  it("does not optimistically move a note before the server accepts it", async () => {
    const { client, wrapper } = setup();
    const note = create(NoteSchema, { name: "notes/moving", space: "spaces/a" });
    client.setQueryData(noteKeys.detail(note.name), note);
    let reject!: (error: Error) => void;
    api.updateNote.mockImplementation(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        }),
    );
    const { result } = renderHook(() => useUpdateNote(), { wrapper });
    act(() => result.current.mutate({ update: { name: note.name, space: "spaces/b" }, updateMask: ["space"] }));
    await waitFor(() => expect(api.updateNote).toHaveBeenCalledOnce());
    expect(client.getQueryData(noteKeys.detail(note.name))).toEqual(note);
    await act(async () => {
      reject(new ConnectError("denied", Code.PermissionDenied));
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(client.getQueryData(noteKeys.detail(note.name))).toEqual(note);
    client.clear();
  });
});

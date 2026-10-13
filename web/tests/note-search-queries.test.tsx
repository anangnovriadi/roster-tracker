import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { buildNoteFilter } from "@/hooks/useNoteFilters";
import { useInfiniteNotes } from "@/hooks/useNoteQueries";
import { shouldRetry } from "@/lib/query-client";

const listNotes = vi.hoisted(() => vi.fn());
vi.mock("@/connect", () => ({ noteServiceClient: { listNotes }, userServiceClient: {} }));

const createWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: shouldRetry, retryDelay: 0 } } });
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe("note search requests", () => {
  beforeEach(() => {
    listNotes.mockReset();
  });

  it.each([
    "content.contains('urgent')",
    'tags.exists(t, t.startsWith("work/"))',
    'content.matches("(?i)urgent")',
    "space == null",
  ])("sends server-supported CEL without interpreting it: %s", async (expression) => {
    listNotes.mockResolvedValue({ notes: [], nextPageToken: "" });
    const filter = buildNoteFilter({ filters: [{ factor: "celSearch", value: expression }], includePinned: false });
    const { result } = renderHook(() => useInfiniteNotes({ filter }), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(listNotes).toHaveBeenCalledWith(expect.objectContaining({ filter: `(${expression})` }), expect.anything());
    expect(result.current.data?.pages[0].notes).toEqual([]);
  });

  it.each([
    Code.InvalidArgument,
    Code.PermissionDenied,
    Code.Unauthenticated,
  ])("does not retry terminal search errors (%i)", async (code) => {
    listNotes.mockRejectedValue(new ConnectError("rejected", code));
    const { result } = renderHook(() => useInfiniteNotes({ filter: "invalid" }), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(listNotes).toHaveBeenCalledOnce();
  });

  it("recovers when the invalid query is edited", async () => {
    listNotes
      .mockRejectedValueOnce(new ConnectError("invalid expression", Code.InvalidArgument))
      .mockResolvedValue({ notes: [], nextPageToken: "" });
    const { result, rerender } = renderHook(({ filter }) => useInfiniteNotes({ filter }), {
      wrapper: createWrapper(),
      initialProps: { filter: "invalid" },
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    rerender({ filter: "pinned" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.error).toBeNull();
    expect(listNotes).toHaveBeenCalledTimes(2);
  });

  it("retains the existing single retry for transient failures", async () => {
    listNotes.mockRejectedValueOnce(new ConnectError("offline", Code.Unavailable)).mockResolvedValue({ notes: [], nextPageToken: "" });
    const { result } = renderHook(() => useInfiniteNotes({ filter: "pinned" }), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(listNotes).toHaveBeenCalledTimes(2);
  });
});

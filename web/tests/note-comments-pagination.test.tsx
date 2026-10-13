import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const listNoteComments = vi.hoisted(() => vi.fn());

vi.mock("@/connect", () => ({
  noteServiceClient: {
    listNoteComments,
  },
}));

import { useInfiniteNoteComments } from "@/hooks/useNoteQueries";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>
);

describe("useInfiniteNoteComments", () => {
  beforeEach(() => {
    listNoteComments.mockReset();
  });

  it("follows nextPageToken and flattens every fetched comment page", async () => {
    listNoteComments
      .mockResolvedValueOnce({
        comments: [{ name: "notes/comment-1" }, { name: "notes/comment-2" }],
        nextPageToken: "page-2",
      })
      .mockResolvedValueOnce({
        comments: [{ name: "notes/comment-3" }],
        nextPageToken: "",
      });

    const { result } = renderHook(() => useInfiniteNoteComments("notes/parent", { pageSize: 2 }), { wrapper });

    await waitFor(() => expect(result.current.data?.map((note) => note.name)).toEqual(["notes/comment-1", "notes/comment-2"]));
    expect(listNoteComments).toHaveBeenNthCalledWith(1, expect.objectContaining({ parent: "notes/parent", pageSize: 2, pageToken: "" }));
    expect(result.current.hasNextPage).toBe(true);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    await waitFor(() =>
      expect(result.current.data?.map((note) => note.name)).toEqual(["notes/comment-1", "notes/comment-2", "notes/comment-3"]),
    );
    expect(listNoteComments).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ parent: "notes/parent", pageSize: 2, pageToken: "page-2" }),
    );
    expect(result.current.hasNextPage).toBe(false);
  });
});

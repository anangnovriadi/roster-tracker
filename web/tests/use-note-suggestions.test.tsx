import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NoteFilter } from "@/contexts/NoteFilterContext";
import { useNoteSuggestions } from "@/hooks/useNoteSuggestions";

const mocks = vi.hoisted(() => ({
  filters: [] as NoteFilter[],
  noteView: undefined as string | undefined,
  views: [] as { name: string; filter: string }[],
}));
vi.mock("@/contexts/NoteFilterContext", () => ({ useNoteFilterContext: () => mocks }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/me" }) }));
vi.mock("@/hooks/useUserQueries", () => ({ useNoteViews: () => ({ data: mocks.views }) }));

describe("useNoteSuggestions", () => {
  beforeEach(() => {
    mocks.filters = [];
    mocks.noteView = undefined;
    mocks.views = [];
  });

  it("suggests a checklist in a saved task view", () => {
    mocks.noteView = "abc";
    mocks.views = [{ name: "users/me/views/abc", filter: "has_task_list && has_incomplete_tasks" }];
    const { result } = renderHook(() => useNoteSuggestions());
    expect(result.current).toEqual([{ id: "checklist", kind: "checklist", source: "view" }]);
  });

  it("resolves the selected View and reacts to filters, selection, and query loading", () => {
    mocks.noteView = "work";
    const { result, rerender } = renderHook(() => useNoteSuggestions());
    expect(result.current).toEqual([]);
    mocks.views = [{ name: "users/me/views/work", filter: '"work" in tags' }];
    rerender();
    expect(result.current.map((suggestion) => suggestion.id)).toEqual(["tag:work"]);
    mocks.filters = [
      { factor: "tagSearch", value: "release" },
      { factor: "celSearch", value: '"planning" in tags' },
    ];
    rerender();
    expect(result.current.map((suggestion) => suggestion.id)).toEqual(["tag:release", "tag:planning", "tag:work"]);
    mocks.noteView = undefined;
    rerender();
    expect(result.current.map((suggestion) => suggestion.id)).toEqual(["tag:release", "tag:planning"]);
  });
});

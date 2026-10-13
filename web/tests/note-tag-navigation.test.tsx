import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Tag } from "@/components/NoteContent/Tag";

const navigateTo = vi.hoisted(() => vi.fn());
const origin = vi.hoisted(() => ({ parentPage: "/" as string }));

vi.mock("@/hooks/useNavigateTo", () => ({
  default: () => navigateTo,
}));

vi.mock("@/components/NoteView/NoteViewContext", () => ({
  useNoteViewContext: () => origin,
}));

vi.mock("@/contexts/NoteFilterContext", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/contexts/NoteFilterContext")>();
  return {
    ...actual,
    useNoteFilterContext: () => ({
      getFiltersByFactor: () => [],
      removeFilter: vi.fn(),
      addFilter: vi.fn(),
    }),
  };
});

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ userTagsSetting: undefined }),
}));

describe("Note tag navigation", () => {
  beforeEach(() => {
    navigateTo.mockClear();
    origin.parentPage = "/";
  });

  it("navigates directly to the global collection from a global detail", () => {
    render(
      <MemoryRouter initialEntries={["/notes/parent"]}>
        <Tag data-tag="work">#work</Tag>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText("#work"));
    expect(navigateTo).toHaveBeenCalledWith("/?filter=tagSearch%3Awork");
  });

  it("returns a creator-scoped tag to that creator's note list", () => {
    origin.parentPage = "/?creator=alice";

    render(
      <MemoryRouter initialEntries={["/notes/parent"]}>
        <Tag data-tag="work">#work</Tag>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByText("#work"));
    expect(navigateTo).toHaveBeenCalledWith("/?creator=alice&filter=tagSearch%3Awork");
  });
});

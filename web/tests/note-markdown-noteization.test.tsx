import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { hasMathSyntax } from "@/components/NoteContent/math";
import { NoteMarkdownRenderer } from "@/components/NoteContent/NoteMarkdownRenderer";

vi.mock("@/components/NoteContent/math", () => ({
  hasMathSyntax: vi.fn(() => false),
}));

describe("<NoteMarkdownRenderer /> memoization", () => {
  beforeEach(() => {
    vi.mocked(hasMathSyntax).mockClear();
  });

  it("does not parse again when only the resolved mention Set identity changes", () => {
    const { rerender } = render(
      <NoteMarkdownRenderer content="Hello @alice" noteName="notes/1" resolvedMentionUsernames={new Set(["alice"])} />,
    );

    rerender(<NoteMarkdownRenderer content="Hello @alice" noteName="notes/1" resolvedMentionUsernames={new Set(["alice"])} />);

    expect(hasMathSyntax).toHaveBeenCalledTimes(1);
  });

  it("renders again when the relevant resolved mentions change", () => {
    const { rerender } = render(
      <NoteMarkdownRenderer content="Hello @alice" noteName="notes/1" resolvedMentionUsernames={new Set<string>()} />,
    );

    rerender(<NoteMarkdownRenderer content="Hello @alice" noteName="notes/1" resolvedMentionUsernames={new Set(["alice"])} />);

    expect(hasMathSyntax).toHaveBeenCalledTimes(2);
    // A resolved mention is styled, but notes are not browsed per member, so it is not a link.
    expect(screen.getByText("@alice")).toHaveAttribute("data-mention", "alice");
    expect(screen.queryByRole("link", { name: "@alice" })).not.toBeInTheDocument();
  });
});

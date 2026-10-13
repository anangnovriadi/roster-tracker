import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import NotePinnedMark from "@/components/NoteView/components/NotePinnedMark";

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

describe("Note pinned mark", () => {
  it("exposes the pinned state without adding an unpin button", () => {
    render(<NotePinnedMark />);
    const mark = screen.getByRole("img", { name: "note.pinned" });
    expect(mark.tagName).toBe("SPAN");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    fireEvent.click(mark);
    expect(mark).toBeInTheDocument();
    expect(mark).not.toHaveAttribute("tabindex", "0");
  });
});

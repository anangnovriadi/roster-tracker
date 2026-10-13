import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ReactionSelector from "@/components/NoteReactionListView/ReactionSelector";
import { NoteSchema } from "@/types/proto/api/note_service_pb";

vi.mock("@/components/NoteReactionListView/hooks", () => ({
  useReactionActions: () => ({
    hasReacted: () => false,
    handleReactionClick: vi.fn(),
  }),
}));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ noteRelatedSetting: { reactions: ["👍"] } }),
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

describe("ReactionSelector", () => {
  it("uses a named native trigger with a visible focus state", async () => {
    const note = create(NoteSchema, { name: "notes/1", reactions: [] });
    render(<ReactionSelector note={note} />);

    const trigger = screen.getByRole("button", { name: "setting.note.add-reaction" });
    // The default trigger is a round quiet 28px face, closing the strip of reaction pills it follows.
    expect(trigger).toHaveClass("size-7", "rounded-full", "focus-visible:outline-2");
    expect(trigger.className).not.toMatch(/ring-/);
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    fireEvent.click(trigger);

    await waitFor(() => expect(trigger).toHaveAttribute("aria-expanded", "true"));
    expect(screen.getByRole("button", { name: "👍" })).toBeInTheDocument();
  });
});

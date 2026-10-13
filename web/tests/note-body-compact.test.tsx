import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CLAMP_PREVIEW_HEIGHT_PX, CLAMP_TRIGGER_HEIGHT_PX } from "@/components/ClampedSection";
import NoteBody from "@/components/NoteView/components/NoteBody";

const mockState = vi.hoisted(() => ({
  note: {
    name: "notes/1",
    content: "",
    relations: [],
    attachments: [],
    reactions: [] as Array<{ reactionType: string; creator: string }>,
  },
  blurred: false,
  showBlurredContent: false,
  toggleBlurVisibility: vi.fn(),
  reactionClick: vi.fn(),
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

vi.mock("@/components/NoteContent/MentionResolutionContext", () => ({
  useResolvedMentionUsernames: () => new Set<string>(),
}));

vi.mock("@/components/NoteContent/NoteMarkdownRenderer", () => ({
  NoteMarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

vi.mock("@/components/NoteMetadata", () => ({
  AttachmentGallery: () => null,
  NoteMetadataRows: () => null,
}));

vi.mock("@/components/NoteReactionListView", () => ({
  NoteReactionListView: ({ note }: { note: { reactions: unknown[] } }) =>
    note.reactions.length > 0 ? (
      <button type="button" aria-label="note-reaction" onClick={mockState.reactionClick}>
        Reaction
      </button>
    ) : null,
}));

vi.mock("@/components/NoteView/hooks", () => ({
  useNoteHandlers: () => ({
    handleNoteContentClick: vi.fn(),
    handleNoteContentDoubleClick: vi.fn(),
  }),
}));

vi.mock("@/components/NoteView/NoteViewContext", () => ({
  useNoteViewContext: () => ({
    note: mockState.note,
    parentPage: "",
    showBlurredContent: mockState.showBlurredContent,
    blurred: mockState.blurred,
    readonly: false,
    openEditor: vi.fn(),
    openPreview: vi.fn(),
    toggleBlurVisibility: mockState.toggleBlurVisibility,
  }),
}));

const createNote = (content: string, reactions: Array<{ reactionType: string; creator: string }> = []) => ({
  name: "notes/1",
  content,
  relations: [],
  attachments: [],
  reactions,
});

afterEach(() => {
  mockState.blurred = false;
  mockState.showBlurredContent = false;
  mockState.toggleBlurVisibility.mockReset();
  mockState.reactionClick.mockReset();
  vi.restoreAllMocks();
});

describe("<NoteBody /> compact body clamp", () => {
  it("keeps a buffer between the preview height and the fold trigger", () => {
    expect(CLAMP_TRIGGER_HEIGHT_PX).toBeGreaterThan(CLAMP_PREVIEW_HEIGHT_PX);
  });

  it("folds tall compact bodies and keeps them expanded across content changes", async () => {
    // jsdom has no layout; report every element as taller than the fold trigger.
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(CLAMP_TRIGGER_HEIGHT_PX + 100);

    mockState.note = createNote("line 1\nline 2");
    const { rerender } = render(<NoteBody compact={true} />);

    await waitFor(() => expect(screen.getByRole("button", { name: /note\.show-more/ })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /note\.show-more/ }));
    expect(screen.getByRole("button", { name: /note\.show-less/ })).toBeInTheDocument();

    mockState.note = createNote("line 1\nline 2 updated");
    rerender(<NoteBody compact={true} />);

    expect(screen.getByRole("button", { name: /note\.show-less/ })).toBeInTheDocument();
  });

  it("renders no clamp when compact is off", () => {
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(CLAMP_TRIGGER_HEIGHT_PX + 100);

    mockState.note = createNote("tall content");
    render(<NoteBody compact={false} />);

    expect(screen.queryByRole("button", { name: /note\.show-more/ })).toBeNull();
  });

  it("renders the sensitive-content action as a button", () => {
    mockState.blurred = true;
    mockState.note = createNote("sensitive content");

    render(<NoteBody compact={false} />);

    const revealButton = screen.getByRole("button", { name: "note.click-to-show-sensitive-content" });
    expect(revealButton.className).not.toMatch(/ring-/);

    fireEvent.click(revealButton);
    expect(mockState.toggleBlurVisibility).toHaveBeenCalledOnce();
  });

  it("scopes the sensitive-content overlay to the note body", () => {
    mockState.blurred = true;
    mockState.note = createNote("sensitive content");

    render(<NoteBody compact={false} />);

    const revealButton = screen.getByRole("button", { name: "note.click-to-show-sensitive-content" });
    const noteBody = revealButton.closest('[data-slot="note-body"]');

    expect(noteBody).not.toBeNull();
    expect(noteBody).toHaveClass("relative", "w-full");
  });

  it("keeps reactions interactive while sensitive content is blurred", () => {
    mockState.blurred = true;
    mockState.note = createNote("sensitive content", [{ reactionType: "👍", creator: "users/alice" }]);

    render(<NoteBody compact={false} />);

    const reaction = screen.getByRole("button", { name: "note-reaction" });
    const blurredContent = screen.getByText("sensitive content").closest(".blur-lg");

    expect(blurredContent).not.toBeNull();
    expect(blurredContent).not.toContainElement(reaction);
    expect(reaction.closest('[data-slot="note-body"]')).toBeNull();

    fireEvent.click(reaction);
    expect(mockState.reactionClick).toHaveBeenCalledOnce();
  });
});

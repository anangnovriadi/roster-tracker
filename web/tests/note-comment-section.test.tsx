import { create } from "@bufbuild/protobuf";
import { render, screen } from "@testing-library/react";
import { act, createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NoteCommentSection, { type NoteCommentSectionHandle } from "@/components/NoteCommentSection";
import type { NoteEditorProps } from "@/components/NoteEditor/types";
import { State } from "@/types/proto/api/common_pb";
import { NoteSchema } from "@/types/proto/api/note_service_pb";

const mocks = vi.hoisted(() => ({
  currentUser: { name: "users/alice" } as { name: string } | undefined,
  loadNoteEditor: vi.fn(),
}));

vi.mock("@/components/NoteEditor/loader", () => ({ loadNoteEditor: mocks.loadNoteEditor }));
vi.mock("@/components/NoteView", () => ({ default: () => <div data-testid="comment-note" /> }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => mocks.currentUser }));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const MockNoteEditor = ({ autoFocus, parentNoteName }: NoteEditorProps) => (
  <div data-testid="comment-editor" data-auto-focus={autoFocus} data-parent-note-name={parentNoteName}>
    <textarea aria-label="Comment editor" />
  </div>
);

describe("NoteCommentSection", () => {
  beforeEach(() => {
    mocks.currentUser = { name: "users/alice" };
    mocks.loadNoteEditor.mockReset();
    mocks.loadNoteEditor.mockResolvedValue({ default: MockNoteEditor });
  });

  it("does not offer or imperatively open a comment editor for archived notes", async () => {
    const note = create(NoteSchema, {
      name: "notes/archived",
      creator: "users/alice",
      state: State.ARCHIVED,
    });

    const ref = createRef<NoteCommentSectionHandle>();
    render(<NoteCommentSection ref={ref} note={note} comments={[]} />);

    await act(async () => {
      await ref.current?.openEditor();
    });

    expect(screen.getByRole("heading", { name: "note.comment.self" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "note.comment.self" })).toHaveAttribute("id", "note-comments");
    expect(screen.queryByRole("button", { name: /note.comment.write-a-comment/ })).not.toBeInTheDocument();
    expect(mocks.loadNoteEditor).not.toHaveBeenCalled();
  });

  it("opens and focuses the existing comment editor through its sidebar handle", async () => {
    const note = create(NoteSchema, {
      name: "notes/detail",
      creator: "users/alice",
      state: State.NORMAL,
    });
    const ref = createRef<NoteCommentSectionHandle>();

    render(
      <>
        <button type="button">Outside editor</button>
        <NoteCommentSection ref={ref} note={note} comments={[]} />
      </>,
    );
    await act(async () => {
      await ref.current?.openEditor();
    });

    expect(mocks.loadNoteEditor).toHaveBeenCalledOnce();
    expect(screen.getByTestId("comment-editor")).toHaveAttribute("data-auto-focus", "true");
    expect(screen.getByTestId("comment-editor")).toHaveAttribute("data-parent-note-name", "notes/detail");

    screen.getByRole("button", { name: "Outside editor" }).focus();
    await act(async () => {
      await ref.current?.openEditor();
    });
    expect(screen.getByRole("textbox", { name: "Comment editor" })).toHaveFocus();
  });

  it("uses the supplied total instead of the currently loaded page length", () => {
    const note = create(NoteSchema, {
      name: "notes/detail",
      creator: "users/alice",
      state: State.NORMAL,
    });
    const comment = create(NoteSchema, { name: "notes/comment" });

    render(<NoteCommentSection note={note} comments={[comment]} commentCount={24} />);

    expect(screen.getByText("(24)")).toBeInTheDocument();
    expect(screen.getAllByTestId("comment-note")).toHaveLength(1);
  });
});

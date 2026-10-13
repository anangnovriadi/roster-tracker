import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TimestampPopover } from "@/components/NoteEditor/components/TimestampPopover";
import { noteService } from "@/components/NoteEditor/services/noteService";
import { createInitialState, EditorProvider, useEditorContext } from "@/components/NoteEditor/state";
import { type Note, NoteSchema } from "@/types/proto/api/note_service_pb";

const clients = vi.hoisted(() => ({ getNote: vi.fn(), updateNote: vi.fn() }));
vi.mock("@/connect", () => ({
  attachmentServiceClient: { createAttachment: vi.fn() },
  noteServiceClient: clients,
}));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const originalCreate = new Date(2020, 0, 1, 10, 20, 30);
const originalUpdate = new Date(2020, 0, 2, 11, 22, 33);
const customTime = new Date(2024, 1, 29, 12, 34, 56);
let editor: ReturnType<typeof useEditorContext>;

function Probe() {
  editor = useEditorContext();
  return <TimestampPopover />;
}

function renderEditor() {
  const note = create(NoteSchema, {
    name: "notes/timestamps",
    content: "Original content",
    createTime: timestampFromDate(originalCreate),
    updateTime: timestampFromDate(originalUpdate),
  });
  clients.getNote.mockResolvedValue(note);
  render(
    <EditorProvider initialEditorState={{ ...createInitialState(), ...noteService.fromNote(note) }}>
      <Probe />
    </EditorProvider>,
  );
  fireEvent.click(screen.getByRole("button"));
  return note;
}

describe("note editor timestamps", () => {
  beforeEach(() => {
    clients.updateNote.mockImplementation(async ({ note }: { note: Note }) => note);
  });

  it.each([
    [0, "createTime", "create_time"],
    [1, "updateTime", "update_time"],
  ] as const)("saves %s after dismissing the focused input with Escape", async (index, field, path) => {
    const note = renderEditor();
    const input = screen.getAllByRole("textbox")[index];
    act(() => input.focus());
    fireEvent.change(input, { target: { value: "2024-02-29 12:34:56" } });
    fireEvent.keyDown(input, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("textbox")).not.toBeInTheDocument());

    await noteService.save(editor.getState(), { noteName: note.name });

    expect(clients.updateNote).toHaveBeenCalledOnce();
    const request = clients.updateNote.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual([path]);
    expect(request.note[field]).toEqual(timestampFromDate(customTime));
  });

  it.each([
    "2024-02-30 12:34:56",
    "2024-13-01 12:34:56",
    "2024-02-29 24:34:56",
    "invalid",
  ])("rejects invalid local time %s without changing the saved timestamp", async (value) => {
    const note = renderEditor();
    const input = screen.getAllByRole("textbox")[0];
    fireEvent.change(input, { target: { value } });
    fireEvent.blur(input);
    expect(input).toHaveAttribute("data-invalid", "true");
    expect(editor.getState().timestamps.createTime).toEqual(originalCreate);
    await noteService.save(editor.getState(), { noteName: note.name });
    expect(clients.updateNote).not.toHaveBeenCalled();
  });

  it("automatically updates the modification time for content edits", async () => {
    const note = renderEditor();
    const state = { ...editor.getState(), content: "Changed content" };
    await noteService.save(state, { noteName: note.name });
    const request = clients.updateNote.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual(["content", "update_time"]);
    expect(request.note.updateTime).toBeUndefined();
    expect(request.note.createTime).toBeUndefined();
  });

  it("keeps the explicitly chosen modification time when content also changes", async () => {
    const note = renderEditor();
    const input = screen.getAllByRole("textbox")[1];
    fireEvent.change(input, { target: { value: "2024-02-29 12:34:56" } });
    // Saving must see the edit even if the input has not blurred yet.
    await noteService.save({ ...editor.getState(), content: "Changed content" }, { noteName: note.name });
    const request = clients.updateNote.mock.calls[0][0];
    expect(request.updateMask.paths).toEqual(["content", "update_time"]);
    expect(request.note.updateTime).toEqual(timestampFromDate(customTime));
  });
});

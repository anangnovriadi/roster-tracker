import { create } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { noteService } from "@/components/NoteEditor/services/noteService";
import { createInitialState, type EditorState } from "@/components/NoteEditor/state";
import { type Note, NoteSchema, Visibility } from "@/types/proto/api/note_service_pb";

const clients = vi.hoisted(() => ({
  getNote: vi.fn(),
  updateNote: vi.fn(),
}));

vi.mock("@/connect", () => ({
  attachmentServiceClient: { createAttachment: vi.fn() },
  noteServiceClient: { getNote: clients.getNote, updateNote: clients.updateNote },
}));

const stored = (fields: Partial<Note> = {}) =>
  create(NoteSchema, { name: "notes/1", content: "Same", visibility: Visibility.PRIVATE, ...fields } as Parameters<typeof create>[1]);

const editedFrom = (note: Note, metadata: Partial<EditorState["metadata"]>): EditorState => {
  const state = createInitialState();
  const initial = noteService.fromNote(note);
  return { ...state, ...initial, metadata: { ...state.metadata, ...initial.metadata, ...metadata } };
};

const savedMask = () => clients.updateNote.mock.calls[0][0].updateMask.paths as string[];
const savedNote = () => clients.updateNote.mock.calls[0][0].note as Note;

describe("editing a note's Space", () => {
  beforeEach(() => {
    clients.getNote.mockReset();
    clients.updateNote.mockReset();
    clients.updateNote.mockImplementation(async ({ note }: { note: Note }) => note);
  });

  it("loads the note's Space into the editor", () => {
    expect(noteService.fromNote(stored({ space: "spaces/work" })).metadata.space).toBe("spaces/work");
    expect(noteService.fromNote(stored()).metadata.space).toBeUndefined();
  });

  it("sends placement and audience together when the Space changes, and reports the move", async () => {
    const note = stored({ space: "spaces/work", visibility: Visibility.SPACE });
    clients.getNote.mockResolvedValue(note);

    const result = await noteService.save(editedFrom(note, { space: "spaces/reading" }), { noteName: note.name });

    expect(new Set(savedMask())).toEqual(new Set(["space", "visibility"]));
    expect(savedNote().space).toBe("spaces/reading");
    expect(savedNote().visibility).toBe(Visibility.SPACE);
    expect(result).toMatchObject({ hasChanges: true, moved: true });
  });

  it("clears the Space with an empty name when the note leaves it", async () => {
    const note = stored({ space: "spaces/work" });
    clients.getNote.mockResolvedValue(note);

    await noteService.save(editedFrom(note, { space: undefined }), { noteName: note.name });

    expect(savedMask()).toContain("space");
    expect(savedNote().space).toBe("");
  });

  it("leaves placement out of the update when the Space is unchanged", async () => {
    const note = stored({ space: "spaces/work" });
    clients.getNote.mockResolvedValue(note);

    const result = await noteService.save({ ...editedFrom(note, {}), content: "Edited" }, { noteName: note.name });

    expect(savedMask()).not.toContain("space");
    expect(result.moved).toBe(false);
  });
});

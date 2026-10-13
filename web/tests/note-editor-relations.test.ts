import { create } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { noteService } from "@/components/NoteEditor/services/noteService";
import { createInitialState, type EditorState } from "@/components/NoteEditor/state";
import {
  type Note,
  NoteRelation_NoteRefSchema,
  NoteRelation_Type,
  NoteRelationSchema,
  NoteSchema,
} from "@/types/proto/api/note_service_pb";

const clients = vi.hoisted(() => ({
  createNote: vi.fn(),
  createNoteComment: vi.fn(),
  getNote: vi.fn(),
  updateNote: vi.fn(),
}));

vi.mock("@/connect", () => ({
  attachmentServiceClient: {
    createAttachment: vi.fn(),
  },
  noteServiceClient: {
    createNote: clients.createNote,
    createNoteComment: clients.createNoteComment,
    getNote: clients.getNote,
    updateNote: clients.updateNote,
  },
}));

const createRelation = (type: NoteRelation_Type, noteName: string, relatedNoteName: string) =>
  create(NoteRelationSchema, {
    type,
    note: create(NoteRelation_NoteRefSchema, { name: noteName }),
    relatedNote: create(NoteRelation_NoteRefSchema, { name: relatedNoteName }),
  });

const createReference = (relatedNoteName: string) =>
  create(NoteRelationSchema, {
    type: NoteRelation_Type.REFERENCE,
    relatedNote: create(NoteRelation_NoteRefSchema, { name: relatedNoteName }),
  });

const createEditorState = (note: Note): EditorState => ({
  ...createInitialState(),
  ...noteService.fromNote(note),
});

describe("note editor relation updates", () => {
  beforeEach(() => {
    clients.createNote.mockReset().mockImplementation(async ({ note }: { note: Note }) => ({ ...note, name: "notes/created" }));
    clients.createNoteComment
      .mockReset()
      .mockImplementation(async ({ comment }: { comment: Note }) => ({ ...comment, name: "notes/comment" }));
    clients.getNote.mockReset();
    clients.updateNote.mockReset();
    clients.updateNote.mockImplementation(async ({ note }: { note: Note }) => note);
  });

  it("assigns a new top-level note to the selected Space", async () => {
    const state = createInitialState();
    state.content = "Roadmap";

    await noteService.save(state, { space: "spaces/product" });

    expect(clients.createNote).toHaveBeenCalledWith({
      note: expect.objectContaining({ content: "Roadmap", space: "spaces/product" }),
    });
  });

  it("saves the draft destination unless the host fixes the Space", async () => {
    const state = createInitialState();
    state.content = "Draft";
    state.metadata = { ...state.metadata, space: "spaces/work" };
    await noteService.save(state, {});
    expect(clients.createNote).toHaveBeenLastCalledWith({ note: expect.objectContaining({ space: "spaces/work" }) });
    await noteService.save(state, { space: "spaces/current" });
    expect(clients.createNote).toHaveBeenLastCalledWith({ note: expect.objectContaining({ space: "spaces/current" }) });
  });

  it("does not inherit the selected Space when creating a comment", async () => {
    const state = createInitialState();
    state.content = "Reply";
    state.metadata = { ...state.metadata, space: "spaces/draft" };

    await noteService.save(state, { parentNoteName: "notes/parent", space: "spaces/product" });

    expect(clients.createNoteComment).toHaveBeenCalledOnce();
    const request = clients.createNoteComment.mock.calls[0][0];
    expect(request.parent).toBe("notes/parent");
    expect(request.comment.content).toBe("Reply");
    expect(request.comment.space).toBeUndefined();
  });

  it("sends only mutable references when editing a comment", async () => {
    const commentName = "notes/comment";
    const parentRelation = createRelation(NoteRelation_Type.COMMENT, commentName, "notes/parent");
    const existingReference = createRelation(NoteRelation_Type.REFERENCE, commentName, "notes/existing-reference");
    const incomingReference = createRelation(NoteRelation_Type.REFERENCE, "notes/referencing-comment", commentName);
    const comment = create(NoteSchema, {
      name: commentName,
      content: "Comment",
      relations: [parentRelation, existingReference, incomingReference],
    });
    const addedReference = createReference("notes/added-reference");
    const state = createEditorState(comment);
    state.metadata.relations = [...state.metadata.relations, addedReference];
    clients.getNote.mockResolvedValue(comment);

    await noteService.save(state, { noteName: commentName });

    expect(clients.updateNote).toHaveBeenCalledOnce();
    const request = clients.updateNote.mock.calls[0][0];
    expect(request.updateMask.paths).toContain("relations");
    expect(request.note.relations).toEqual([existingReference, addedReference]);
    expect(request.note.relations.every((relation: { type: NoteRelation_Type }) => relation.type === NoteRelation_Type.REFERENCE)).toBe(
      true,
    );
  });

  it("ignores a new incoming comment when comparing a parent note's relations", async () => {
    const parentName = "notes/parent";
    const outgoingReference = createRelation(NoteRelation_Type.REFERENCE, parentName, "notes/reference");
    const editorNote = create(NoteSchema, {
      name: parentName,
      content: "Parent",
      relations: [outgoingReference],
    });
    const freshNote = create(NoteSchema, {
      name: parentName,
      content: editorNote.content,
      relations: [outgoingReference, createRelation(NoteRelation_Type.COMMENT, "notes/new-comment", parentName)],
    });
    const state = createEditorState(editorNote);
    clients.getNote.mockResolvedValue(freshNote);

    const result = await noteService.save(state, { noteName: parentName });

    expect(result).toEqual({ noteName: parentName, hasChanges: false });
    expect(clients.updateNote).not.toHaveBeenCalled();
  });
});

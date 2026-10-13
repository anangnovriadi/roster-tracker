import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema, timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt";
import { isEqual } from "lodash-es";
import { getEditorReferenceRelations } from "@/components/NoteMetadata/Relation/relationHelpers";
import { noteServiceClient } from "@/connect";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import type { Note } from "@/types/proto/api/note_service_pb";
import { NoteSchema, Visibility } from "@/types/proto/api/note_service_pb";
import type { EditorState } from "../state";
import type { LocalFile } from "../types/attachment";
import { uploadService } from "./uploadService";

/**
 * Converts attachments to reference format for API requests.
 * The backend only needs the attachment name to link it to a note.
 */
function toAttachmentReferences(attachments: Attachment[]): Attachment[] {
  return attachments.map((a) => create(AttachmentSchema, { name: a.name }));
}

function buildUpdateMask(
  prevNote: Note,
  state: EditorState,
  allAttachments: typeof state.metadata.attachments,
): { mask: Set<string>; patch: Partial<Note> } {
  const mask = new Set<string>();
  const patch: Partial<Note> = {
    name: prevNote.name,
    content: state.content,
  };

  if (!isEqual(state.content, prevNote.content)) {
    mask.add("content");
    patch.content = state.content;
  }
  if (!isEqual(state.metadata.visibility, prevNote.visibility)) {
    mask.add("visibility");
    patch.visibility = state.metadata.visibility;
  }
  // Placement and audience are validated together: a move always states the
  // audience it lands with, so a Space-only audience never follows it implicitly.
  if ((state.metadata.space || undefined) !== (prevNote.space || undefined)) {
    mask.add("space");
    mask.add("visibility");
    patch.space = state.metadata.space ?? "";
    patch.visibility = state.metadata.visibility;
  }
  if (!isEqual(allAttachments, prevNote.attachments)) {
    mask.add("attachments");
    patch.attachments = toAttachmentReferences(allAttachments);
  }
  const previousReferenceRelations = getEditorReferenceRelations(prevNote.relations, prevNote.name);
  const nextReferenceRelations = getEditorReferenceRelations(state.metadata.relations, prevNote.name);
  if (!isEqual(nextReferenceRelations, previousReferenceRelations)) {
    mask.add("relations");
    patch.relations = nextReferenceRelations;
  }
  if (!isEqual(state.metadata.location, prevNote.location)) {
    mask.add("location");
    patch.location = state.metadata.location;
  }

  // Auto-update timestamp if content changed
  if (["content", "attachments", "relations", "location"].some((key) => mask.has(key))) {
    mask.add("update_time");
  }

  // Handle custom timestamps
  if (state.timestamps.createTime) {
    const prevCreateTime = prevNote.createTime ? timestampDate(prevNote.createTime) : undefined;
    if (!isEqual(state.timestamps.createTime, prevCreateTime)) {
      mask.add("create_time");
      patch.createTime = timestampFromDate(state.timestamps.createTime);
    }
  }
  if (state.timestamps.updateTime) {
    const prevUpdateTime = prevNote.updateTime ? timestampDate(prevNote.updateTime) : undefined;
    if (!isEqual(state.timestamps.updateTime, prevUpdateTime)) {
      mask.add("update_time");
      patch.updateTime = timestampFromDate(state.timestamps.updateTime);
    }
  }

  return { mask, patch };
}

export const noteService = {
  async save(
    state: EditorState,
    options: {
      noteName?: string;
      parentNoteName?: string;
      space?: string;
      /** Runs as each local file finishes uploading, before the note is saved. */
      onFileUploaded?: (localFile: LocalFile, attachment: Attachment) => void;
    },
  ): Promise<{ noteName: string; hasChanges: boolean; moved?: boolean }> {
    // 1. Upload local files first
    const newAttachments = await uploadService.uploadFiles(state.localFiles, options.onFileUploaded);
    const allAttachments = [...state.metadata.attachments, ...newAttachments];

    // 2. Update existing note
    if (options.noteName) {
      const prevNote = await noteServiceClient.getNote({ name: options.noteName });
      const { mask, patch } = buildUpdateMask(prevNote, state, allAttachments);

      if (mask.size === 0) {
        return { noteName: prevNote.name, hasChanges: false };
      }

      const note = await noteServiceClient.updateNote({
        note: create(NoteSchema, patch as Record<string, unknown>),
        updateMask: create(FieldMaskSchema, { paths: Array.from(mask) }),
      });
      return { noteName: note.name, hasChanges: true, moved: mask.has("space") };
    }

    // 3. Create new note or comment
    const noteData = create(NoteSchema, {
      content: state.content,
      visibility: Visibility.PUBLIC,
      attachments: toAttachmentReferences(allAttachments),
      relations: state.metadata.relations,
      location: state.metadata.location,
      createTime: state.timestamps.createTime ? timestampFromDate(state.timestamps.createTime) : undefined,
      updateTime: state.timestamps.updateTime ? timestampFromDate(state.timestamps.updateTime) : undefined,
      space: options.parentNoteName ? undefined : (options.space ?? state.metadata.space),
    });

    const note = options.parentNoteName
      ? await noteServiceClient.createNoteComment({
          parent: options.parentNoteName,
          comment: noteData,
        })
      : await noteServiceClient.createNote({ note: noteData });

    return { noteName: note.name, hasChanges: true };
  },

  /**
   * Build the INIT_NOTE payload from an already-loaded Note entity (no network
   * request). Returns only the fields the reducer's INIT_NOTE case consumes —
   * UI state (mode, loading flags, …) is owned by the reducer, not by notes.
   */
  fromNote(note: Note): Pick<EditorState, "content" | "metadata" | "timestamps"> {
    return {
      content: note.content,
      metadata: {
        visibility: note.visibility,
        space: note.space || undefined,
        attachments: note.attachments,
        relations: note.relations,
        location: note.location,
      },
      timestamps: {
        createTime: note.createTime ? timestampDate(note.createTime) : undefined,
        updateTime: note.updateTime ? timestampDate(note.updateTime) : undefined,
      },
    };
  },
};

import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { estimateNoteCardHeight } from "@/components/PagedNoteList/noteCardHeight";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { NoteRelation_NoteRefSchema, NoteRelation_Type, NoteRelationSchema, NoteSchema } from "@/types/proto/api/note_service_pb";

const buildAttachment = (overrides: MessageInitShape<typeof AttachmentSchema>) =>
  create(AttachmentSchema, {
    name: "attachments/test",
    filename: "test.bin",
    type: "application/octet-stream",
    ...overrides,
  });

const buildCommentRelation = (noteName: string, index: number) =>
  create(NoteRelationSchema, {
    type: NoteRelation_Type.COMMENT,
    note: create(NoteRelation_NoteRefSchema, { name: `notes/comment-${index}` }),
    relatedNote: create(NoteRelation_NoteRefSchema, { name: noteName }),
  });

const buildNote = (overrides: MessageInitShape<typeof NoteSchema> = {}) =>
  create(NoteSchema, {
    name: "notes/main",
    content: "hello",
    attachments: [],
    relations: [],
    ...overrides,
  });

describe("estimateNoteCardHeight", () => {
  it("accounts for visual attachments before images have loaded", () => {
    const plain = buildNote();
    const withImage = buildNote({
      attachments: [
        buildAttachment({
          name: "attachments/image",
          filename: "image.png",
          type: "image/png",
        }),
      ],
    });

    expect(estimateNoteCardHeight(withImage, { columnWidth: 320 })).toBeGreaterThan(
      estimateNoteCardHeight(plain, { columnWidth: 320 }) + 100,
    );
  });

  it("does not count an inline managed image again as an attachment gallery", () => {
    const image = buildAttachment({ name: "attachments/image", filename: "image.png", type: "image/png" });
    const inline = buildNote({ content: "![image](/file/attachments/image)", attachments: [image] });
    const external = buildNote({ content: "![image](https://example.com/image.png)", attachments: [image] });

    expect(estimateNoteCardHeight(inline, { columnWidth: 320 })).toBeLessThan(estimateNoteCardHeight(external, { columnWidth: 320 }));
  });

  it("estimates the visible comment preview height and caps it at three comments", () => {
    const oneComment = buildNote({ relations: [buildCommentRelation("notes/main", 1)] });
    const threeComments = buildNote({
      relations: [1, 2, 3].map((index) => buildCommentRelation("notes/main", index)),
    });
    const fiveComments = buildNote({
      relations: [1, 2, 3, 4, 5].map((index) => buildCommentRelation("notes/main", index)),
    });

    expect(estimateNoteCardHeight(threeComments, { columnWidth: 320 })).toBeGreaterThan(
      estimateNoteCardHeight(oneComment, { columnWidth: 320 }),
    );
    expect(estimateNoteCardHeight(fiveComments, { columnWidth: 320 })).toBe(estimateNoteCardHeight(threeComments, { columnWidth: 320 }));
  });
});

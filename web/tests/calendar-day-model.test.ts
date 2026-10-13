import { create, type MessageInitShape } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { describe, expect, it } from "vitest";
import { buildCalendarMonthModel, getNoteThumbnail } from "@/components/CalendarView/dayModel";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { type Note, NoteSchema } from "@/types/proto/api/note_service_pb";

const noteAt = (hour: number, overrides: MessageInitShape<typeof NoteSchema> = {}): Note =>
  create(NoteSchema, {
    name: `notes/${hour}`,
    createTime: timestampFromDate(new Date(2026, 7, 2, hour)),
    updateTime: timestampFromDate(new Date(2026, 7, 3, hour)),
    ...overrides,
  });
const image = (name: string) => create(AttachmentSchema, { name: `attachments/${name}`, filename: `${name}.jpg`, type: "image/jpeg" });
const pdf = create(AttachmentSchema, { name: "attachments/doc", filename: "doc.pdf", type: "application/pdf" });
const day = (notes: Note[]) => buildCalendarMonthModel(notes, "create_time")["2026-08-02"];

describe("calendar day snapshots", () => {
  it("groups by local day and keeps every note in chronological order", () => {
    const notes = Array.from({ length: 24 }, (_, i) => noteAt(i, { content: `note ${i}` })).reverse();
    const summary = day(notes);
    expect(summary.notes).toHaveLength(24);
    expect(summary.notes[0].name).toBe("notes/0");
    expect(summary.excerpt?.text).toBe("note 0");
  });

  it("uses note identity to choose the same sample when timestamps tie", () => {
    const a = noteAt(9, { name: "notes/a", content: "first" });
    const b = noteAt(9, { name: "notes/b", content: "second" });
    expect(day([b, a]).excerpt).toEqual(day([a, b]).excerpt);
    expect(day([b, a]).excerpt?.text).toBe("first");
  });

  it("uses full Markdown content instead of a truncated snippet, without markup or image captions", () => {
    const summary = day([
      noteAt(9, {
        content: "# Morning\n\nA **quiet** walk with [a friend](https://example.com).\n\n![caption](/photo.jpg)",
        snippet: "Morning…",
      }),
    ]);
    expect(summary.excerpt?.text).toBe("Morning\nA quiet walk with a friend.");
    expect(summary.excerpt?.isCode).toBe(false);
  });

  it("preserves readable code and checklist text without fences or active checkboxes", () => {
    expect(day([noteAt(9, { content: "```js\nconst x = 1;\nconsole.log(x);\n```" })]).excerpt).toMatchObject({
      text: "const x = 1;\nconsole.log(x);",
      isCode: true,
    });
    expect(day([noteAt(9, { content: "- [x] Ship release\n- [ ] Write notes" })]).excerpt?.text).toBe("Ship release\nWrite notes");
  });

  it("skips empty and image-only notes when choosing text and keeps photos from distinct notes", () => {
    const summary = day([
      noteAt(8),
      noteAt(9, { attachments: [image("a"), image("b")] }),
      noteAt(10, { content: "The day's writing", attachments: [image("c")] }),
      noteAt(11, { attachments: [image("d")] }),
    ]);
    expect(summary.excerpt?.text).toBe("The day's writing");
    expect(summary.images.map((entry) => entry.noteName)).toEqual(["notes/9", "notes/10"]);
    expect(summary.images[0].thumbnailUrl).toContain("attachments/a/a.jpg?thumbnail=true");
    expect(summary.notes).toHaveLength(4);
  });

  it("does not duplicate an inline attachment or a repeated image", () => {
    const photo = image("a");
    expect(
      day([
        noteAt(9, { content: "![photo](/file/attachments/a/a.jpg)", attachments: [photo] }),
        noteAt(10, { attachments: [photo] }),
        noteAt(11, { attachments: [image("b")] }),
      ]).images,
    ).toHaveLength(2);
  });

  it("prefers Markdown images over attachment order and supports local SVG references", () => {
    const svg = create(AttachmentSchema, { name: "attachments/bird", filename: "bird.svg", type: "image/svg+xml" });
    const result = day([noteAt(9, { content: "![bird](/file/attachments/bird)", attachments: [image("a"), svg] })]);
    expect(result.images[0].thumbnailUrl).toContain("attachments/bird/bird.svg?thumbnail=true");
    expect(result.images).toHaveLength(1);
  });

  it("extracts external and reference-style images in content order", () => {
    const result = day([
      noteAt(9, {
        content:
          "![first][PHOTO]\n\n![second](https://example.com/second.jpg)\n\n[photo]: https://example.com/first.jpg\n[photo]: https://example.com/ignored.jpg",
        attachments: [image("a")],
      }),
      noteAt(10, { content: "![next](https://example.com/next.jpg)" }),
    ]);
    expect(result.images.map((entry) => entry.thumbnailUrl)).toEqual(["https://example.com/first.jpg", "https://example.com/next.jpg"]);
  });

  it("deduplicates content and attachment images before filling the next day slot", () => {
    const photo = image("a");
    const result = day([
      noteAt(9, { content: "![a](/file/attachments/a)", attachments: [photo] }),
      noteAt(10, { content: "![a](/file/attachments/a/a.jpg)\n\n![b](https://example.com/b.jpg)", attachments: [photo] }),
    ]);
    expect(result.images).toHaveLength(2);
    expect(result.images[1].thumbnailUrl).toBe("https://example.com/b.jpg");
  });

  it("deduplicates equivalent external URLs", () => {
    const result = day([
      noteAt(9, { content: "![a](https://EXAMPLE.com/a.jpg)" }),
      noteAt(10, { content: "![a](https://example.com/a.jpg)\n\n![b](https://example.com/b.jpg)" }),
    ]);
    expect(result.images.map((entry) => entry.thumbnailUrl)).toEqual(["https://EXAMPLE.com/a.jpg", "https://example.com/b.jpg"]);
  });

  it("ignores code examples, raw HTML, unresolved references, and disallowed image URLs", () => {
    const result = day([
      noteAt(9, {
        content: [
          "`![code](https://example.com/code.jpg)`",
          "```md\n![fenced](https://example.com/fenced.jpg)\n```",
          '<img src="https://example.com/html.jpg">',
          "![missing][undefined]",
          "![data](data:image/png;base64,AAAA)",
          "![insecure](http://example.com/insecure.jpg)",
          "![script](javascript:alert)",
          "![managed](/file/attachments/missing)",
          "![valid](https://example.com/valid.jpg)",
        ].join("\n\n"),
      }),
    ]);
    expect(result.images.map((entry) => entry.thumbnailUrl)).toEqual(["https://example.com/valid.jpg"]);
  });

  it("does not turn an image-only note's alt text into an excerpt", () => {
    const result = day([noteAt(9, { content: "![caption](https://example.com/a.jpg)", snippet: "caption" })]);
    expect(result.excerpt).toBeUndefined();
    expect(result.images).toHaveLength(1);
  });

  it("uses a filename only when the day has no readable text", () => {
    expect(day([noteAt(9, { attachments: [pdf] })]).excerpt?.text).toBe("doc.pdf");
    expect(day([noteAt(9, { attachments: [pdf] }), noteAt(10, { content: "Notes" })]).excerpt?.text).toBe("Notes");
  });

  it("attributes the excerpt to the note it came from, filename fallback included", () => {
    const text = day([
      noteAt(9, { creator: "users/bob", content: "Morning run" }),
      noteAt(10, { creator: "users/alice", content: "Lunch" }),
    ]);
    expect(text.excerpt).toMatchObject({ noteName: "notes/9", creator: "users/bob", text: "Morning run" });
    expect(day([noteAt(9, { creator: "users/carol", attachments: [pdf] })]).excerpt).toMatchObject({
      creator: "users/carol",
      text: "doc.pdf",
    });
  });

  it("names each creator once, in the order they first wrote, hidden notes included", () => {
    const summary = buildCalendarMonthModel(
      [
        noteAt(11, { creator: "users/bob" }),
        noteAt(9, { creator: "users/carol", tags: ["private"] }),
        noteAt(10, { creator: "users/bob" }),
        noteAt(12, { creator: "users/alice" }),
      ],
      "create_time",
      { isRedacted: (note) => note.tags.includes("private") },
    )["2026-08-02"];
    expect(summary.creators).toEqual(["users/carol", "users/bob", "users/alice"]);
  });

  it("counts hidden notes without exposing text, filenames, or photos", () => {
    const summary = buildCalendarMonthModel(
      [
        noteAt(9, { tags: ["private"], content: "secret ![private](https://example.com/secret.jpg)", attachments: [pdf, image("secret")] }),
        noteAt(10, { content: "visible" }),
      ],
      "create_time",
      { isRedacted: (note) => note.tags.includes("private") },
    )["2026-08-02"];
    expect(summary.notes).toHaveLength(2);
    expect(summary.excerpt?.text).toBe("visible");
    expect(summary.images).toEqual([]);
  });

  it("uses update time when selected and ignores undated notes", () => {
    expect(Object.keys(buildCalendarMonthModel([noteAt(9), create(NoteSchema)], "update_time"))).toEqual(["2026-08-03"]);
  });

  it("finds a single note's first photo in the same order, for map pins", () => {
    expect(getNoteThumbnail(noteAt(9, { content: "just words" }))).toBeUndefined();
    expect(getNoteThumbnail(noteAt(9, { attachments: [pdf] }))).toBeUndefined();
    expect(getNoteThumbnail(noteAt(9, { content: "![a](https://example.com/inline.jpg)", attachments: [image("later")] }))).toBe(
      "https://example.com/inline.jpg",
    );
    expect(getNoteThumbnail(noteAt(9, { attachments: [image("only")] }))).toBe(
      day([noteAt(9, { attachments: [image("only")] })]).images[0].thumbnailUrl,
    );
  });
});

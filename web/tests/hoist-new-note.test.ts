import { describe, expect, it } from "vitest";
import { hoistNoteToFront } from "@/hooks/useNoteSorting";
import type { Note } from "@/types/proto/api/note_service_pb";

const note = (name: string) => ({ name }) as Note;

describe("hoistNoteToFront", () => {
  it("moves the named note to the front, above everything else", () => {
    const list = [note("notes/pin1"), note("notes/pin2"), note("notes/new")];
    expect(hoistNoteToFront(list, "notes/new").map((m) => m.name)).toEqual(["notes/new", "notes/pin1", "notes/pin2"]);
  });

  it("returns the list unchanged when the name is null", () => {
    const list = [note("notes/a"), note("notes/b")];
    expect(hoistNoteToFront(list, null)).toBe(list);
  });

  it("returns the list unchanged when the name is absent", () => {
    const list = [note("notes/a"), note("notes/b")];
    expect(hoistNoteToFront(list, "notes/missing")).toBe(list);
  });

  it("returns the list unchanged when the note is already first", () => {
    const list = [note("notes/a"), note("notes/b")];
    expect(hoistNoteToFront(list, "notes/a")).toBe(list);
  });

  it("does not mutate the input list", () => {
    const list = [note("notes/a"), note("notes/new")];
    hoistNoteToFront(list, "notes/new");
    expect(list.map((m) => m.name)).toEqual(["notes/a", "notes/new"]);
  });
});

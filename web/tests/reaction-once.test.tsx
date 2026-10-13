import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { hasReactedToNote } from "@/components/NoteReactionListView/hooks";
import { NoteSchema, ReactionSchema } from "@/types/proto/api/note_service_pb";

describe("hasReactedToNote", () => {
  const note = create(NoteSchema, {
    name: "notes/1",
    reactions: [create(ReactionSchema, { creator: "users/alice", reactionType: "👍" })],
  });

  it("is true only for a member who already reacted", () => {
    expect(hasReactedToNote(note, "users/alice")).toBe(true);
    expect(hasReactedToNote(note, "users/bob")).toBe(false);
    expect(hasReactedToNote(note, undefined)).toBe(false);
  });
});

import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { hasReactedToMemo } from "@/components/MemoReactionListView/hooks";
import { MemoSchema, ReactionSchema } from "@/types/proto/api/memo_service_pb";

describe("hasReactedToMemo", () => {
  const memo = create(MemoSchema, {
    name: "memos/1",
    reactions: [create(ReactionSchema, { creator: "users/alice", reactionType: "👍" })],
  });

  it("is true only for a member who already reacted", () => {
    expect(hasReactedToMemo(memo, "users/alice")).toBe(true);
    expect(hasReactedToMemo(memo, "users/bob")).toBe(false);
    expect(hasReactedToMemo(memo, undefined)).toBe(false);
  });
});

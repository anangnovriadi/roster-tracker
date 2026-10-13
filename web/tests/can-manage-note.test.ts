import { describe, expect, it } from "vitest";
import { User, User_Role } from "@/types/proto/api/user_service_pb";
import { canManageNote, canWriteNotes } from "@/utils/user";

const user = (name: string, role: User_Role) => ({ name, role }) as User;

describe("canManageNote", () => {
  const note = { creator: "users/alice" };

  it("allows the creator", () => {
    expect(canManageNote(note, user("users/alice", User_Role.USER))).toBe(true);
  });

  it("denies an instance admin, who only reads notes", () => {
    expect(canManageNote(note, user("users/root", User_Role.ADMIN))).toBe(false);
    expect(canWriteNotes(user("users/root", User_Role.ADMIN))).toBe(false);
    expect(canWriteNotes(user("users/bob", User_Role.USER))).toBe(true);
    expect(canWriteNotes(undefined)).toBe(false);
  });

  it("denies other users and anonymous viewers", () => {
    expect(canManageNote(note, user("users/bob", User_Role.USER))).toBe(false);
    expect(canManageNote(note, undefined)).toBe(false);
  });
});

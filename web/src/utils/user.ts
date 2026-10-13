import { User, User_Role } from "@/types/proto/api/user_service_pb";

export const isSuperUser = (user: User | undefined) => {
  return user && user.role === User_Role.ADMIN;
};

/**
 * Whether the user may write notes at all: add notes, comment, react, and edit. Admins run
 * the instance and the roster and only read notes; the server refuses their note writes.
 */
export const canWriteNotes = (user: User | undefined): user is User => !!user && !isSuperUser(user);

/** Whether the user holds author-level authority over a note: only its creator, and never an admin. */
export const canManageNote = (note: { creator: string }, user: User | undefined): boolean => {
  return canWriteNotes(user) && note.creator === user.name;
};

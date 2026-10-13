import { User, User_Role } from "@/types/proto/api/user_service_pb";

export const isSuperUser = (user: User | undefined) => {
  return user && user.role === User_Role.ADMIN;
};

/**
 * Whether the user holds author-level authority over a note: its creator, or an
 * instance admin, who is the superuser for named note operations on the server.
 */
export const canManageNote = (note: { creator: string }, user: User | undefined): boolean => {
  return !!user && (note.creator === user.name || !!isSuperUser(user));
};

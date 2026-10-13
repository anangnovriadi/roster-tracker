import { useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { useResolvedUsersByNames } from "@/components/NoteContent/MentionResolutionContext";
import { noteServiceClient } from "@/connect";
import useCurrentUser from "@/hooks/useCurrentUser";
import { noteKeys } from "@/hooks/useNoteQueries";
import type { Note, Reaction } from "@/types/proto/api/note_service_pb";
import type { User } from "@/types/proto/api/user_service_pb";

export type ReactionGroup = Map<string, User[]>;

export const useReactionGroups = (reactions: Reaction[]): ReactionGroup => {
  const creatorNames = useMemo(() => reactions.map((r) => r.creator), [reactions]);
  const userMap = useResolvedUsersByNames(creatorNames);

  return useMemo(() => {
    const reactionGroup = new Map<string, User[]>();
    for (const reaction of reactions) {
      const user = userMap?.get(reaction.creator);
      if (!user) continue;

      const users = reactionGroup.get(reaction.reactionType) || [];
      users.push(user);
      reactionGroup.set(reaction.reactionType, users);
    }
    return reactionGroup;
  }, [reactions, userMap]);
};

interface UseReactionActionsOptions {
  note: Note;
  onComplete?: () => void;
}

export const useReactionActions = ({ note, onComplete }: UseReactionActionsOptions) => {
  const currentUser = useCurrentUser();
  const queryClient = useQueryClient();

  const hasReacted = (reactionType: string) => {
    return note.reactions.some((r) => r.reactionType === reactionType && r.creator === currentUser?.name);
  };

  const handleReactionClick = async (reactionType: string) => {
    if (!currentUser) return;

    try {
      if (hasReacted(reactionType)) {
        const reactions = note.reactions.filter(
          (reaction) => reaction.reactionType === reactionType && reaction.creator === currentUser.name,
        );
        await Promise.all(reactions.map((reaction) => noteServiceClient.deleteNoteReaction({ name: reaction.name })));
      } else {
        await noteServiceClient.upsertNoteReaction({
          parent: note.name,
          reaction: { reactionType },
        });
      }
      // Refetch the note to get updated reactions and invalidate cache
      const updatedNote = await noteServiceClient.getNote({ name: note.name });
      queryClient.setQueryData(noteKeys.detail(note.name), updatedNote);
      queryClient.invalidateQueries({ queryKey: noteKeys.lists() });
      // If this note is a comment, refresh the parent's comments list so the comment's reactions update in the UI
      if (note.parent) {
        queryClient.invalidateQueries({ queryKey: noteKeys.comments(note.parent) });
      }
    } catch {
      // skip error
    }
    onComplete?.();
  };

  return { hasReacted, handleReactionClick };
};

/** Each member reacts to a note at most once. */
export const hasReactedToNote = (note: Note, userName?: string): boolean =>
  Boolean(userName) && (note.reactions ?? []).some((reaction) => reaction.creator === userName);

export const formatReactionTooltip = (users: User[], reactionType: string): string => {
  if (users.length === 0) return "";
  const formatUserName = (user: User) => user.displayName || user.username;
  if (users.length < 5) {
    return `${users.map(formatUserName).join(", ")} reacted with ${reactionType.toLowerCase()}`;
  }
  return `${users.slice(0, 4).map(formatUserName).join(", ")} and ${users.length - 4} more reacted with ${reactionType.toLowerCase()}`;
};

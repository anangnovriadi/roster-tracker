import { memo } from "react";
import useCurrentUser from "@/hooks/useCurrentUser";
import { State } from "@/types/proto/api/common_pb";
import type { Note, Reaction } from "@/types/proto/api/note_service_pb";
import { hasReactedToNote, useReactionGroups } from "./hooks";
import ReactionSelector from "./ReactionSelector";
import ReactionView from "./ReactionView";

interface Props {
  note: Note;
  reactions: Reaction[];
}

const NoteReactionListView = (props: Props) => {
  const { note: noteData, reactions } = props;
  const currentUser = useCurrentUser();
  const reactionGroup = useReactionGroups(reactions);
  const readonly = noteData.state === State.ARCHIVED;

  if (reactions.length === 0) {
    return null;
  }

  return (
    // Chips are filled surfaces, so their box edge, not their padding, sits on the text edge.
    <div className="flex w-full flex-row flex-wrap items-center justify-start gap-1 select-none">
      {Array.from(reactionGroup).map(([reactionType, users]) => (
        <ReactionView key={`${reactionType.toString()} ${users.length}`} note={noteData} reactionType={reactionType} users={users} />
      ))}
      {!readonly && currentUser && !hasReactedToNote(noteData, currentUser.name) && <ReactionSelector note={noteData} />}
    </div>
  );
};

export default memo(NoteReactionListView);

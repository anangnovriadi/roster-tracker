import { memo } from "react";
import type { Note, Reaction } from "@/types/proto/api/note_service_pb";
import { useReactionGroups } from "./hooks";
import ReactionView from "./ReactionView";

interface Props {
  note: Note;
  reactions: Reaction[];
}

const NoteReactionListView = (props: Props) => {
  const { note: noteData, reactions } = props;
  const reactionGroup = useReactionGroups(reactions);

  if (reactions.length === 0) {
    return null;
  }

  return (
    // Adding a reaction lives in the card header; this row only lists the reactions.
    // Chips are filled surfaces, so their box edge, not their padding, sits on the text edge.
    <div className="flex w-full flex-row flex-wrap items-center justify-start gap-1 select-none">
      {Array.from(reactionGroup).map(([reactionType, users]) => (
        <ReactionView key={`${reactionType.toString()} ${users.length}`} note={noteData} reactionType={reactionType} users={users} />
      ))}
    </div>
  );
};

export default memo(NoteReactionListView);

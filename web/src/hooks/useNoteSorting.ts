import { timestampDate } from "@bufbuild/protobuf/wkt";
import dayjs from "dayjs";
import { useMemo } from "react";
import { type NoteTimeBasis, useView } from "@/contexts/ViewContext";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";

export interface UseNoteSortingOptions {
  pinnedFirst?: boolean;
  state?: State;
}

export interface UseNoteSortingResult {
  listSort: (notes: Note[]) => Note[];
  orderBy: string;
}

/** The timestamp a note is ordered and bucketed by under `timeBasis`. */
export const getNoteSortTime = (note: Note, timeBasis: NoteTimeBasis): Date | undefined => {
  const timestamp = timeBasis === "update_time" ? note.updateTime : note.createTime;
  return timestamp ? timestampDate(timestamp) : undefined;
};

// Moves the note with the given name to the front of the list, above pinned
// notes, so a freshly created note is immediately visible. Returns the input
// unchanged when the name is null, absent, or already first.
export const hoistNoteToFront = (notes: Note[], name: string | null): Note[] => {
  if (!name) return notes;
  const index = notes.findIndex((note) => note.name === name);
  if (index <= 0) return notes;
  const reordered = notes.slice();
  const [note] = reordered.splice(index, 1);
  reordered.unshift(note);
  return reordered;
};

export const useNoteSorting = (options: UseNoteSortingOptions = {}): UseNoteSortingResult => {
  const { pinnedFirst = false, state = State.NORMAL } = options;
  const { orderByTimeAsc, timeBasis } = useView();

  // Generate orderBy string for API
  const orderBy = useMemo(() => {
    const timeOrder = orderByTimeAsc ? `${timeBasis} asc` : `${timeBasis} desc`;
    return pinnedFirst ? `pinned desc, ${timeOrder}` : timeOrder;
  }, [pinnedFirst, orderByTimeAsc, timeBasis]);

  // Generate listSort function for client-side sorting
  const listSort = useMemo(() => {
    return (notes: Note[]): Note[] => {
      return notes
        .filter((note) => note.state === state)
        .sort((a, b) => {
          // First, sort by pinned status if enabled
          if (pinnedFirst && a.pinned !== b.pinned) {
            return b.pinned ? 1 : -1;
          }

          // Then sort by the selected time field.
          const aTime = getNoteSortTime(a, timeBasis);
          const bTime = getNoteSortTime(b, timeBasis);
          return orderByTimeAsc ? dayjs(aTime).unix() - dayjs(bTime).unix() : dayjs(bTime).unix() - dayjs(aTime).unix();
        });
    };
  }, [pinnedFirst, state, orderByTimeAsc, timeBasis]);

  return { listSort, orderBy };
};

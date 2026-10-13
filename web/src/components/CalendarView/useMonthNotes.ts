import { create } from "@bufbuild/protobuf";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { noteServiceClient } from "@/connect";
import { useView } from "@/contexts/ViewContext";
import { noteKeys } from "@/hooks/useNoteQueries";
import { getLocalMonthTimestampRange, withTimestampRange } from "@/lib/calendar-utils";
import { State } from "@/types/proto/api/common_pb";
import { ListNotesRequestSchema, type Note } from "@/types/proto/api/note_service_pb";
import { type BuildCalendarMonthModelOptions, buildCalendarMonthModel } from "./dayModel";

/** A month usually fits one page; the loop below still drains any that do not. */
const MONTH_PAGE_SIZE = 500;
const NO_NOTES: Note[] = [];

export interface UseMonthNotesOptions extends BuildCalendarMonthModelOptions {
  /** `YYYY-MM` */
  month: string;
  /** CEL clauses fixing whose notes and which collection; the month range is added here. */
  filter?: string;
  enabled?: boolean;
}

/** Every page of the month: a partial month would understate counts and drop rows. */
const listWholeMonth = async (filter: string | undefined, orderBy: string): Promise<Note[]> => {
  const notes: Note[] = [];
  let pageToken = "";
  do {
    const response = await noteServiceClient.listNotes(
      create(ListNotesRequestSchema, { state: State.NORMAL, filter, orderBy, pageSize: MONTH_PAGE_SIZE, pageToken }),
    );
    notes.push(...response.notes);
    pageToken = response.nextPageToken;
  } while (pageToken);
  return notes;
};

/**
 * Every note of one month, folded into per-day summaries for the grid. Lives under the note
 * list cache key so the create/update/delete mutations invalidate it like any other list.
 */
export const useMonthNotes = ({ month, filter, enabled = true, isRedacted }: UseMonthNotesOptions) => {
  const { timeBasis } = useView();
  const monthFilter = withTimestampRange(filter, getLocalMonthTimestampRange(month), timeBasis);
  const orderBy = `${timeBasis} asc`;

  const query = useQuery({
    queryKey: [...noteKeys.lists(), "calendar-month", { filter: monthFilter, orderBy }],
    queryFn: () => listWholeMonth(monthFilter, orderBy),
    enabled,
    staleTime: 1000 * 60,
  });

  const notes = query.data ?? NO_NOTES;
  const model = useMemo(() => buildCalendarMonthModel(notes, timeBasis, { isRedacted }), [notes, timeBasis, isRedacted]);
  return { model, isLoading: query.isLoading, error: query.isError ? query.error : undefined, refetch: query.refetch };
};

import { useMemo } from "react";
import { type NoteFilter, useNoteFilterContext } from "@/contexts/NoteFilterContext";
import { type NoteTimeBasis, useView } from "@/contexts/ViewContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useNoteViews } from "@/hooks/useUserQueries";
import { buildTimestampRangeFilter, getLocalDayTimestampRange, getTimeBasisField } from "@/lib/calendar-utils";
import { combineCELFilters } from "@/lib/cel-filter";
import { getNoteViewId } from "@/lib/note-views";
import { buildNoteCreatorFilter, getVisibilityName } from "@/lib/resource-names";
import { Visibility } from "@/types/proto/api/note_service_pb";

const escapeFilterValue = (value: string): string => JSON.stringify(value);

export interface UseNoteFiltersOptions {
  creatorName?: string;
  includeNoteViews?: boolean;
  includePinned?: boolean;
  visibilities?: Visibility[];
}

interface BuildNoteFilterOptions {
  creatorName?: string;
  filters: NoteFilter[];
  includePinned: boolean;
  selectedNoteViewFilter?: string;
  visibilities?: Visibility[];
  /** Which timestamp a `displayTime` day filter selects on; defaults to creation time. */
  timeBasis?: NoteTimeBasis;
}

export const buildNoteFilter = ({
  creatorName,
  filters,
  includePinned,
  selectedNoteViewFilter,
  visibilities,
  timeBasis = "create_time",
}: BuildNoteFilterOptions): string | undefined => {
  const conditions: string[] = [];

  if (creatorName) {
    const creatorFilter = buildNoteCreatorFilter(creatorName);
    if (creatorFilter) {
      conditions.push(creatorFilter);
    }
  }

  if (selectedNoteViewFilter) {
    conditions.push(selectedNoteViewFilter);
  }

  for (const filter of filters) {
    if (filter.factor === "contentSearch") {
      conditions.push(`content.contains(${escapeFilterValue(filter.value)})`);
    } else if (filter.factor === "celSearch") {
      conditions.push(filter.value);
    } else if (filter.factor === "tagSearch") {
      conditions.push(`tag in [${escapeFilterValue(filter.value)}]`);
    } else if (filter.factor === "pinned") {
      if (includePinned) {
        conditions.push(`pinned`);
      }
    } else if (filter.factor === "property.hasLink") {
      conditions.push(`has_link`);
    } else if (filter.factor === "property.hasTaskList") {
      conditions.push(`has_task_list`);
    } else if (filter.factor === "property.hasCode") {
      conditions.push(`has_code`);
    } else if (filter.factor === "property.hasLocation") {
      conditions.push(`has_location`);
    } else if (filter.factor === "displayTime") {
      const range = getLocalDayTimestampRange(filter.value);
      if (range) {
        conditions.push(buildTimestampRangeFilter(getTimeBasisField(timeBasis), range));
      }
    }
  }

  if (visibilities && visibilities.length > 0) {
    const visibilityValues = visibilities.map((visibility) => `"${getVisibilityName(visibility)}"`).join(", ");
    conditions.push(`visibility in [${visibilityValues}]`);
  }

  return combineCELFilters(...conditions);
};

/** Stored CEL filter of the saved View selected in the sidebar. */
export const useSelectedNoteViewFilter = (): string | undefined => {
  const currentUser = useCurrentUser();
  const { noteView } = useNoteFilterContext();
  const { data: noteViews = [] } = useNoteViews(currentUser?.name);
  return useMemo(() => {
    return noteViews.find((view) => getNoteViewId(view.name) === noteView)?.filter;
  }, [noteView, noteViews]);
};

export const useNoteFilters = (options: UseNoteFiltersOptions = {}): string | undefined => {
  const { creatorName, includeNoteViews = false, includePinned = false, visibilities } = options;

  const { filters } = useNoteFilterContext();
  const selectedNoteViewFilter = useSelectedNoteViewFilter();
  // The sidebar calendar buckets days by this basis, so a picked day must select on it too.
  const { timeBasis } = useView();

  return useMemo(
    () =>
      buildNoteFilter({
        creatorName,
        filters,
        includePinned,
        selectedNoteViewFilter: includeNoteViews ? selectedNoteViewFilter : undefined,
        visibilities,
        timeBasis,
      }),
    [creatorName, filters, includePinned, includeNoteViews, selectedNoteViewFilter, visibilities, timeBasis],
  );
};

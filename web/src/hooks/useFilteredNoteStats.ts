import { timestampDate } from "@bufbuild/protobuf/wkt";
import dayjs from "dayjs";
import { countBy } from "lodash-es";
import { useMemo } from "react";
import { type NoteTimeBasis, useView } from "@/contexts/ViewContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useAllUserStats, useUserStats } from "@/hooks/useUserQueries";
import { mergeTagCounts } from "@/lib/tag";
import { State } from "@/types/proto/api/common_pb";
import type { UserStats } from "@/types/proto/api/user_service_pb";
import type { StatisticsData } from "@/types/statistics";

export interface FilteredNoteStats {
  statistics: StatisticsData;
  tags: Record<string, number>;
  loading: boolean;
}

export type NoteStatsContext = "home" | "collection" | "archived";

export interface UseFilteredNoteStatsOptions {
  userName?: string;
  context?: NoteStatsContext;
  enabled?: boolean;
  filter?: string;
}

const toDateString = (date: Date) => dayjs(date).format("YYYY-MM-DD");

const timestampsForBasis = (stats: UserStats, basis: NoteTimeBasis) => {
  const createdArray = stats.noteCreatedTimestamps ?? [];
  const updatedArray = stats.noteUpdatedTimestamps ?? [];
  const wantUpdated = basis === "update_time";
  const oldServerFallback = wantUpdated && updatedArray.length === 0 && createdArray.length > 0;
  if (oldServerFallback) {
    console.warn("UserStats.note_updated_timestamps not present; falling back to note_created_timestamps");
  }
  return wantUpdated && !oldServerFallback ? updatedArray : createdArray;
};

export const useFilteredNoteStats = (options: UseFilteredNoteStatsOptions = {}): FilteredNoteStats => {
  const { userName, context, enabled = true, filter } = options;
  const currentUser = useCurrentUser();
  const { timeBasis } = useView();

  // Legacy personal collections use backend per-user stats.
  const { data: userStats, isLoading: isLoadingUserStats } = useUserStats(userName, { enabled, filter });
  // Collection and Archived fetch grouped stats and aggregate them locally.
  // ListUserStats AND's the request filter with the server's auth filter, so
  // private notes are not included unless explicitly visible to the current user.
  const allUserStatsRequest =
    context === "collection" ? { state: State.NORMAL, filter } : context === "archived" ? { state: State.ARCHIVED, filter } : {};
  const shouldFetchAllUserStats = context === "collection" || (context === "archived" && !!currentUser?.name);
  const { data: allUserStats = [], isLoading: isLoadingAllUserStats } = useAllUserStats(allUserStatsRequest, {
    enabled: enabled && shouldFetchAllUserStats,
  });

  const data = useMemo(() => {
    const loading = isLoadingUserStats || isLoadingAllUserStats;
    let activityStats: Record<string, number> = {};
    let tagCount: Record<string, number> = mergeTagCounts();

    if (context === "collection" || context === "archived") {
      const displayDates: string[] = [];
      tagCount = mergeTagCounts(...allUserStats.map((stats) => stats.tagCounts));
      for (const stats of allUserStats) {
        displayDates.push(
          ...timestampsForBasis(stats, timeBasis)
            .map((ts) => (ts ? timestampDate(ts) : undefined))
            .filter((date): date is Date => date !== undefined)
            .map(toDateString),
        );
      }
      activityStats = countBy(displayDates);
    } else if (userName && userStats) {
      // Legacy personal collections use backend per-user stats.
      const sourceArray = timestampsForBasis(userStats, timeBasis);
      if (sourceArray.length > 0) {
        activityStats = countBy(
          sourceArray
            .map((ts) => (ts ? timestampDate(ts) : undefined))
            .filter((date): date is Date => date !== undefined)
            .map(toDateString),
        );
      }
      if (userStats.tagCounts) {
        tagCount = mergeTagCounts(userStats.tagCounts);
      }
    }

    return { statistics: { activityStats, timeBasis }, tags: tagCount, loading };
  }, [context, userName, userStats, allUserStats, isLoadingUserStats, isLoadingAllUserStats, timeBasis]);

  return data;
};

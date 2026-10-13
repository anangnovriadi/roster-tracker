import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { rosterServiceClient } from "@/connect";
import { type RosterAssignment, RosterAssignmentSchema } from "@/types/proto/api/roster_service_pb";

export const rosterKeys = {
  all: ["rosterAssignments"] as const,
  range: (startDate: string, endDate: string) => [...rosterKeys.all, startDate, endDate] as const,
};

const EMPTY_ASSIGNMENTS: RosterAssignment[] = [];

/** Formats a local calendar day as the roster's YYYY-MM-DD date. */
export const toRosterDate = (date: Date): string => {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
};

/** Assignments between two YYYY-MM-DD dates, inclusive. Readable without signing in. */
export function useRosterAssignments(startDate: string, endDate: string, enabled = true) {
  const query = useQuery({
    queryKey: rosterKeys.range(startDate, endDate),
    queryFn: async () => (await rosterServiceClient.listRosterAssignments({ startDate, endDate })).rosterAssignments,
    enabled: enabled && Boolean(startDate && endDate),
    staleTime: 60_000,
  });
  return { ...query, assignments: query.data ?? EMPTY_ASSIGNMENTS };
}

/** Groups assignments by their YYYY-MM-DD date. */
export const groupRosterByDate = (assignments: RosterAssignment[]): Map<string, RosterAssignment[]> => {
  const byDate = new Map<string, RosterAssignment[]>();
  for (const assignment of assignments) {
    const list = byDate.get(assignment.date);
    if (list) list.push(assignment);
    else byDate.set(assignment.date, [assignment]);
  }
  return byDate;
};

export function useCreateRosterAssignment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ date, user }: { date: string; user: string }) =>
      rosterServiceClient.createRosterAssignment({ rosterAssignment: create(RosterAssignmentSchema, { date, user }) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: rosterKeys.all }),
  });
}

export function useDeleteRosterAssignment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => rosterServiceClient.deleteRosterAssignment({ name }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: rosterKeys.all }),
  });
}

/** How the roster names a member: "Full Name (@username)", or "@username" without a full name. */
export const formatRosterMember = (userName: string, user?: { displayName?: string; username?: string }): string => {
  const username = user?.username || userName.replace(/^users\//, "");
  return user?.displayName ? `${user.displayName} (@${username})` : `@${username}`;
};

const DAYS_IN_WEEK = 7;

/**
 * The seven YYYY-MM-DD dates of the week holding `date`, starting on the instance's first
 * weekday (`weekStartDayOffset`: 0 is Sunday), the same weeks the calendar draws.
 */
export const rosterWeekDates = (date: string, weekStartDayOffset: number): string[] => {
  const [year, month, day] = date.split("-").map(Number);
  const start = new Date(year, month - 1, day);
  const offset = ((weekStartDayOffset % DAYS_IN_WEEK) + DAYS_IN_WEEK) % DAYS_IN_WEEK;
  start.setDate(start.getDate() - ((start.getDay() - offset + DAYS_IN_WEEK) % DAYS_IN_WEEK));
  return Array.from({ length: DAYS_IN_WEEK }, (_, index) => {
    const next = new Date(start);
    next.setDate(start.getDate() + index);
    return toRosterDate(next);
  });
};

/**
 * Puts one member on duty for every given day, replacing whoever covered those days:
 * other members' assignments are removed first, then the member fills each day.
 */
export function useReplaceRosterDays() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ dates, user, existing }: { dates: string[]; user: string; existing: RosterAssignment[] }) => {
      const days = new Set(dates);
      const kept = new Set<string>();
      for (const assignment of existing) {
        if (!days.has(assignment.date)) continue;
        if (assignment.user === user) {
          kept.add(assignment.date);
          continue;
        }
        await rosterServiceClient.deleteRosterAssignment({ name: assignment.name });
      }
      for (const date of dates) {
        if (kept.has(date)) continue;
        try {
          await rosterServiceClient.createRosterAssignment({ rosterAssignment: create(RosterAssignmentSchema, { date, user }) });
        } catch (error) {
          // Someone else filled the day meanwhile; the next roster refresh shows who.
          if (!(error instanceof ConnectError && error.code === Code.AlreadyExists)) throw error;
        }
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: rosterKeys.all }),
  });
}

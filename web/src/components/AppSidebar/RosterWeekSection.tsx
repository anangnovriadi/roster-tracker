import { LoaderCircleIcon } from "lucide-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { buildCalendarPath, getMonthOfDate } from "@/components/CalendarView/paths";
import { useInstance } from "@/contexts/InstanceContext";
import { formatRosterMember, groupRosterByDate, rosterWeekDates, toRosterDate, useRosterAssignments } from "@/hooks/useRosterQueries";
import { useUsersByNames } from "@/hooks/useUserQueries";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import SidebarSection from "./SidebarSection";

/** Who is on duty each day of the current week; a day opens its page in the calendar. */
const RosterWeekSection = ({ onSelect }: { onSelect?: () => void }) => {
  const t = useTranslate();
  const { i18n } = useTranslation();
  const { generalSetting } = useInstance();
  const today = toRosterDate(new Date());
  const weekDates = useMemo(() => rosterWeekDates(today, generalSetting.weekStartDayOffset), [today, generalSetting.weekStartDayOffset]);
  const { assignments, isLoading } = useRosterAssignments(weekDates[0], weekDates[weekDates.length - 1]);
  const byDate = useMemo(() => groupRosterByDate(assignments), [assignments]);
  const { data: users } = useUsersByNames(useMemo(() => assignments.map((assignment) => assignment.user), [assignments]));

  const weekdayLabel = (date: string) => {
    const [year, month, day] = date.split("-").map(Number);
    return new Date(year, month - 1, day).toLocaleDateString(i18n.language, { weekday: "short", day: "numeric" });
  };

  return (
    <SidebarSection label={t("roster.this-week")}>
      {isLoading ? (
        <p role="status" className="flex items-center gap-2 px-2 py-1 text-ui text-muted-foreground">
          <LoaderCircleIcon className="size-3.5 animate-spin" aria-hidden />
          {t("roster.loading")}
        </p>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {weekDates.map((date) => {
            const onDuty = byDate.get(date) ?? [];
            const isToday = date === today;
            return (
              <li key={date}>
                <Link
                  to={buildCalendarPath(getMonthOfDate(date), date)}
                  onClick={onSelect}
                  aria-current={isToday ? "date" : undefined}
                  className={cn(
                    "flex min-h-7 items-baseline gap-2 rounded-md px-2 py-1 text-ui hover:bg-sidebar-accent/65",
                    isToday && "bg-sidebar-accent font-medium",
                  )}
                >
                  <span className={cn("w-14 shrink-0 text-muted-foreground", isToday && "text-primary")}>{weekdayLabel(date)}</span>
                  <span className={cn("min-w-0 flex-1 truncate", onDuty.length === 0 && "text-muted-foreground/60")}>
                    {onDuty.length === 0
                      ? "—"
                      : onDuty.map((assignment) => formatRosterMember(assignment.user, users?.get(assignment.user))).join(", ")}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </SidebarSection>
  );
};

export default RosterWeekSection;

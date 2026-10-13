import { CalendarCheckIcon, LoaderCircleIcon } from "lucide-react";
import { useMemo } from "react";
import UserAvatar from "@/components/UserAvatar";
import { formatRosterMember, toRosterDate, useRosterAssignments } from "@/hooks/useRosterQueries";
import { useUsersByNames } from "@/hooks/useUserQueries";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

/** Names who is on duty today, so every visitor sees the roster before the notes. */
const RosterTodayBanner = ({ className }: { className?: string }) => {
  const t = useTranslate();
  const today = toRosterDate(new Date());
  const { assignments, isLoading } = useRosterAssignments(today, today);
  const userNames = useMemo(() => assignments.map((assignment) => assignment.user), [assignments]);
  const { data: users } = useUsersByNames(userNames);

  return (
    <section
      aria-label={t("roster.on-duty-today")}
      className={cn(
        "mb-3 flex flex-col gap-2 rounded-lg border border-primary/30 bg-primary/5 px-4 py-3 sm:flex-row sm:items-center",
        className,
      )}
    >
      <span className="flex shrink-0 items-center gap-2 text-sm font-semibold text-foreground">
        <CalendarCheckIcon className="size-4 text-primary" />
        {t("roster.on-duty-today")}
      </span>
      {isLoading ? (
        <span role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" aria-hidden />
          {t("roster.loading")}
        </span>
      ) : assignments.length === 0 ? (
        <span className="text-sm text-muted-foreground">{t("roster.nobody-today")}</span>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {assignments.map((assignment) => {
            const user = users?.get(assignment.user);
            const name = formatRosterMember(assignment.user, user);
            return (
              <li
                key={assignment.name}
                className="flex items-center gap-1.5 rounded-full border border-border bg-background py-0.5 ps-1 pe-2.5 text-sm"
              >
                <UserAvatar className="size-5" avatarUrl={user?.avatarUrl} name={name} />
                {name}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
};

export default RosterTodayBanner;

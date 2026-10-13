import { LoaderCircleIcon, SearchIcon, SearchXIcon, UserRoundCheckIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { buildCalendarPath, getMonthOfDate } from "@/components/CalendarView/paths";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { formatRosterMember, toRosterDate, useRosterAssignments } from "@/hooks/useRosterQueries";
import { useUsersByNames } from "@/hooks/useUserQueries";
import { useTranslate } from "@/utils/i18n";

/** How far back and ahead the roster search looks; the API lists at most a year at a time. */
const DAYS_BEFORE = 30;
const DAYS_AFTER = 335;

const shiftDate = (date: Date, days: number) => {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
};

const normalize = (value: string) =>
  value
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();

/** The sidebar's Search: finds the days a member is on duty, by full name or username. */
const QuickFindDialog = () => {
  const t = useTranslate();
  const { i18n } = useTranslation();
  const navigate = useNavigate();
  const { quickFindOpen, setQuickFindOpen } = useAppSidebar();
  const [query, setQuery] = useState("");
  const today = new Date();
  const startDate = toRosterDate(shiftDate(today, -DAYS_BEFORE));
  const endDate = toRosterDate(shiftDate(today, DAYS_AFTER));
  const { assignments, isLoading } = useRosterAssignments(startDate, endDate, quickFindOpen);
  const userNames = useMemo(() => assignments.map((assignment) => assignment.user), [assignments]);
  const { data: users } = useUsersByNames(userNames, { enabled: quickFindOpen });

  useEffect(() => {
    if (quickFindOpen) setQuery("");
  }, [quickFindOpen]);

  const needle = normalize(query.trim());
  const results = useMemo(() => {
    if (!needle) return [];
    return assignments
      .map((assignment) => ({ assignment, label: formatRosterMember(assignment.user, users?.get(assignment.user)) }))
      .filter(({ label }) => normalize(label).includes(needle));
  }, [assignments, users, needle]);

  const formatDate = (date: string) => {
    const [year, month, day] = date.split("-").map(Number);
    return new Date(year, month - 1, day).toLocaleDateString(i18n.language, {
      weekday: "short",
      day: "numeric",
      month: "short",
      year: "numeric",
    });
  };

  const openDay = (date: string) => {
    setQuickFindOpen(false);
    navigate(buildCalendarPath(getMonthOfDate(date), date));
  };

  return (
    <Dialog open={quickFindOpen} onOpenChange={setQuickFindOpen}>
      <DialogContent
        size="lg"
        className="top-[12vh]! translate-y-0! overflow-hidden border-border/70 p-0! shadow-xl sm:top-[16vh]! [&>div:first-child]:gap-0!"
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{t("roster.search")}</DialogTitle>
        <DialogDescription className="sr-only">{t("roster.search-placeholder")}</DialogDescription>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (results.length > 0) openDay(results[0].assignment.date);
          }}
        >
          <div className="flex h-[52px] shrink-0 items-center gap-2.5 border-b border-border/50 px-4">
            <SearchIcon className="size-[17px] shrink-0 text-muted-foreground" strokeWidth={1.8} />
            <Input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              className="h-10 border-0 bg-transparent px-0 !text-[14px] shadow-none focus-visible:ring-0"
              placeholder={t("roster.search-placeholder")}
              aria-label={t("roster.search")}
            />
          </div>
        </form>
        <div className="max-h-[50dvh] overflow-y-auto p-2">
          {!needle ? (
            <p className="px-2 py-6 text-center text-sm text-muted-foreground">{t("roster.search-hint")}</p>
          ) : isLoading ? (
            <p role="status" className="flex items-center justify-center gap-2 px-2 py-6 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" aria-hidden />
              {t("roster.loading")}
            </p>
          ) : results.length === 0 ? (
            <div role="status" className="flex flex-col items-center gap-2 px-2 py-8 text-center">
              <SearchXIcon className="size-8 text-muted-foreground/60" strokeWidth={1.5} aria-hidden />
              <p className="text-sm font-medium text-foreground">{t("roster.not-found")}</p>
              <p className="text-xs text-muted-foreground">{t("roster.not-found-description", { query: query.trim() })}</p>
            </div>
          ) : (
            <ul aria-label={t("roster.search-results")} className="flex flex-col gap-0.5">
              {results.map(({ assignment, label }) => (
                <li key={assignment.name}>
                  <button
                    type="button"
                    onClick={() => openDay(assignment.date)}
                    className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-start text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  >
                    <UserRoundCheckIcon className="size-4 shrink-0 text-primary" aria-hidden />
                    <span className="w-40 shrink-0 text-muted-foreground">{formatDate(assignment.date)}</span>
                    <span className="min-w-0 flex-1 truncate">{label}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};

export default QuickFindDialog;

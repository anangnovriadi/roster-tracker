import { Code } from "@connectrpc/connect";
import { sortBy } from "lodash-es";
import { LoaderCircleIcon, PlusIcon, UserRoundCheckIcon, XIcon } from "lucide-react";
import { useMemo, useState } from "react";
import toast from "react-hot-toast";
import { useTranslation } from "react-i18next";
import UserAvatar from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useInstance } from "@/contexts/InstanceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import {
  formatRosterMember,
  rosterWeekDates,
  useCreateRosterAssignment,
  useDeleteRosterAssignment,
  useReplaceRosterDays,
  useRosterAssignments,
} from "@/hooks/useRosterQueries";
import { useListUsers, useUsersByNames } from "@/hooks/useUserQueries";
import { handleError, hasConnectCode } from "@/lib/error";
import { cn } from "@/lib/utils";
import { State } from "@/types/proto/api/common_pb";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";

/**
 * Who is on duty on one calendar day. Everyone can read it; an admin also puts members
 * on the roster and takes them off it here, so the roster is managed where it is shown.
 */
const formatShortDate = (date: string, locale: string) => {
  const [year, month, day] = date.split("-").map(Number);
  return new Date(year, month - 1, day).toLocaleDateString(locale, { day: "numeric", month: "short" });
};

const RosterDayPanel = ({ date }: { date: string }) => {
  const t = useTranslate();
  const { i18n } = useTranslation();
  const currentUser = useCurrentUser();
  const isAdmin = currentUser?.role === User_Role.ADMIN;
  const [member, setMember] = useState("");
  const [mode, setMode] = useState<"day" | "week">("day");
  const { generalSetting } = useInstance();
  const { assignments, isLoading } = useRosterAssignments(date, date);
  const assignedNames = useMemo(() => assignments.map((assignment) => assignment.user), [assignments]);
  const { data: assignedUsers } = useUsersByNames(assignedNames);
  const { data: users = [] } = useListUsers(isAdmin);
  const createAssignment = useCreateRosterAssignment();
  const replaceWeek = useReplaceRosterDays();
  const isAdding = createAssignment.isPending || replaceWeek.isPending;
  const weekDates = useMemo(() => rosterWeekDates(date, generalSetting.weekStartDayOffset), [date, generalSetting.weekStartDayOffset]);
  const { assignments: weekAssignments, isLoading: weekLoading } = useRosterAssignments(
    weekDates[0],
    weekDates[weekDates.length - 1],
    isAdmin && mode === "week",
  );
  const weekRange = `${formatShortDate(weekDates[0], i18n.language)} – ${formatShortDate(weekDates[weekDates.length - 1], i18n.language)}`;
  const deleteAssignment = useDeleteRosterAssignment();

  const memberOptions = useMemo(
    () =>
      sortBy(
        users.filter((user) => user.state === State.NORMAL && (mode === "week" || !assignedNames.includes(user.name))),
        "username",
      ).map((user) => ({ value: user.name, label: formatRosterMember(user.name, user) })),
    [users, assignedNames, mode],
  );

  const displayName = (name: string) => {
    const user = assignedUsers?.get(name) ?? users.find((candidate) => candidate.name === name);
    return formatRosterMember(name, user);
  };

  const handleAdd = () => {
    if (!member) return;
    if (mode === "week") {
      replaceWeek.mutate(
        { dates: weekDates, user: member, existing: weekAssignments },
        {
          onSuccess: () => {
            setMember("");
            toast.success(
              t("roster.week-replaced", {
                name: formatRosterMember(
                  member,
                  users.find((user) => user.name === member),
                ),
              }),
            );
          },
          onError: (error) => handleError(error, toast.error, { context: "Replace roster week" }),
        },
      );
      return;
    }
    createAssignment.mutate(
      { date, user: member },
      {
        onSuccess: () => setMember(""),
        onError: (error) => {
          if (hasConnectCode(error, Code.AlreadyExists)) {
            toast.error(t("roster.already-assigned"));
            return;
          }
          handleError(error, toast.error, { context: "Add roster assignment" });
        },
      },
    );
  };

  const handleRemove = (name: string) => {
    deleteAssignment.mutate(name, {
      onError: (error) => handleError(error, toast.error, { context: "Remove roster assignment" }),
    });
  };

  return (
    <section aria-label={t("roster.on-duty")} className="mb-4 flex flex-col gap-2 rounded-lg border border-primary/30 bg-primary/5 p-3">
      <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
        <UserRoundCheckIcon className="size-4 text-primary" />
        {t("roster.on-duty")}
      </h3>
      {isLoading ? (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" aria-hidden />
          {t("roster.loading")}
        </p>
      ) : assignments.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("roster.nobody-on-day")}</p>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {assignments.map((assignment) => {
            const name = displayName(assignment.user);
            const user = assignedUsers?.get(assignment.user);
            return (
              <li
                key={assignment.name}
                className="flex items-center gap-1.5 rounded-full border border-border bg-background py-0.5 ps-1 pe-2 text-sm"
              >
                <UserAvatar className="size-5" avatarUrl={user?.avatarUrl} name={name} />
                {name}
                {isAdmin && (
                  <button
                    type="button"
                    className="-me-1 rounded-full p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
                    aria-label={t("roster.remove", { name })}
                    onClick={() => handleRemove(assignment.name)}
                  >
                    <XIcon className="size-3.5" />
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {/* One member is on duty per day: a taken day offers no way to add anyone, in either mode,
          until its member is removed. */}
      {isAdmin && !isLoading && (
        <div className="flex flex-col gap-2 border-t border-primary/20 pt-2">
          <div role="group" aria-label={t("roster.assign-mode")} className="grid grid-cols-2 gap-0.5 rounded-md bg-muted p-0.5">
            {(["day", "week"] as const).map((value) => (
              <button
                key={value}
                type="button"
                aria-pressed={mode === value}
                onClick={() => setMode(value)}
                className={cn(
                  "h-7 rounded-sm text-ui font-medium text-muted-foreground transition-colors hover:text-foreground",
                  mode === value && "bg-background text-foreground shadow-sm ring-1 ring-border",
                )}
              >
                {t(value === "day" ? "roster.per-day" : "roster.per-week")}
              </button>
            ))}
          </div>
          {mode === "week" && <p className="text-xs text-muted-foreground">{t("roster.week-range", { range: weekRange })}</p>}
          {/* One member per day: a taken day is changed by removing its member, or by replacing the week. */}
          {mode === "day" && assignments.length > 0 ? (
            <p className="text-xs text-muted-foreground">{t("roster.day-taken")}</p>
          ) : (
            <div className="flex items-center gap-2">
              <Select value={member} items={memberOptions} onValueChange={(value) => setMember(value ?? "")}>
                <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t("roster.select-member")}>
                  <SelectValue placeholder={t("roster.select-member")} />
                </SelectTrigger>
                <SelectContent>
                  {memberOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button size="sm" onClick={handleAdd} disabled={!member || isAdding || (mode === "week" && weekLoading)}>
                <PlusIcon className="size-3.5" />
                {t(mode === "day" ? "roster.add" : "roster.add-week")}
              </Button>
            </div>
          )}
        </div>
      )}
    </section>
  );
};

export default RosterDayPanel;

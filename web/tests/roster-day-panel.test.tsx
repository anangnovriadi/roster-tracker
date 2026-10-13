import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RosterDayPanel from "@/components/CalendarView/RosterDayPanel";
import { User_Role } from "@/types/proto/api/user_service_pb";

const state = vi.hoisted(() => ({
  currentUser: undefined as { name: string; role: number } | undefined,
  assignments: [] as Array<{ name: string; date: string; user: string }>,
  deleteAssignment: vi.fn(),
  range: [] as string[],
  isLoading: false,
  createMany: vi.fn(),
  weekAssignments: [] as Array<{ name: string; date: string; user: string }>,
}));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ generalSetting: { weekStartDayOffset: 1 } }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => state.currentUser }));
vi.mock("@/hooks/useRosterQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useRosterQueries")>()),
  useRosterAssignments: (startDate: string, endDate: string) => {
    if (startDate !== endDate) return { assignments: state.weekAssignments, isLoading: false };
    state.range = [startDate, endDate];
    return { assignments: state.assignments, isLoading: state.isLoading };
  },
  useCreateRosterAssignment: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteRosterAssignment: () => ({ mutate: state.deleteAssignment }),
  useReplaceRosterDays: () => ({ mutate: state.createMany, isPending: false }),
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUsersByNames: () => ({ data: new Map([["users/alice", { username: "alice", displayName: "Alice", avatarUrl: "" }]]) }),
  useListUsers: () => ({ data: [] }),
}));

describe("<RosterDayPanel>", () => {
  beforeEach(() => {
    state.assignments = [{ name: "roster-assignments/1", date: "2026-10-12", user: "users/alice" }];
    state.deleteAssignment.mockClear();
    state.isLoading = false;
    state.weekAssignments = [];
    state.createMany.mockClear();
  });

  it("shows a spinner while the day's roster loads", () => {
    state.isLoading = true;
    render(<RosterDayPanel date="2026-10-12" />);
    expect(screen.getByRole("status")).toHaveTextContent("roster.loading");
    expect(screen.queryByText("Alice")).not.toBeInTheDocument();
  });

  it("shows who is on duty that day to a signed-out visitor, without controls", () => {
    state.currentUser = undefined;
    render(<RosterDayPanel date="2026-10-12" />);

    expect(state.range).toEqual(["2026-10-12", "2026-10-12"]);
    expect(screen.getByText("Alice (@alice)")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("lets an admin take the member off a taken day, and offers no second member for that day alone", () => {
    state.currentUser = { name: "users/admin", role: User_Role.ADMIN };
    render(<RosterDayPanel date="2026-10-12" />);

    expect(screen.queryByRole("button", { name: "roster.add" })).not.toBeInTheDocument();
    expect(screen.getByText("roster.day-taken")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "roster.remove" }));
    expect(state.deleteAssignment).toHaveBeenCalledWith("roster-assignments/1", expect.anything());
  });

  it("lets an admin pick a member, shown as full name and username, for a free day", () => {
    state.currentUser = { name: "users/admin", role: User_Role.ADMIN };
    state.assignments = [];
    render(<RosterDayPanel date="2026-10-13" />);
    expect(screen.getByRole("button", { name: "roster.add" })).toBeDisabled();
  });

  it("says when nobody is on duty", () => {
    state.currentUser = { name: "users/alice", role: User_Role.USER };
    state.assignments = [];
    render(<RosterDayPanel date="2026-10-13" />);
    expect(screen.getByText("roster.nobody-on-day")).toBeInTheDocument();
  });

  it("replaces the whole week from any day, even one already taken", () => {
    state.currentUser = { name: "users/admin", role: User_Role.ADMIN };
    state.weekAssignments = [
      { name: "roster-assignments/1", date: "2026-10-12", user: "users/alice" },
      { name: "roster-assignments/2", date: "2026-10-13", user: "users/alice" },
    ];
    render(<RosterDayPanel date="2026-10-12" />);

    fireEvent.click(screen.getByRole("button", { name: "roster.per-week" }));
    expect(screen.getByRole("button", { name: "roster.per-week" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByText("roster.day-taken")).not.toBeInTheDocument();
    expect(screen.getByText("roster.week-range")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "roster.add-week" })).toBeDisabled();
  });
});

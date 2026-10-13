import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RosterTodayBanner from "@/components/RosterTodayBanner";

const state = vi.hoisted(() => ({
  assignments: [] as Array<{ name: string; date: string; user: string }>,
  range: [] as string[],
  isLoading: false,
}));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/hooks/useRosterQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useRosterQueries")>()),
  useRosterAssignments: (startDate: string, endDate: string) => {
    state.range = [startDate, endDate];
    return { assignments: state.assignments, isLoading: state.isLoading };
  },
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUsersByNames: () => ({ data: new Map([["users/alice", { username: "alice", displayName: "Alice", avatarUrl: "" }]]) }),
}));

describe("<RosterTodayBanner>", () => {
  beforeEach(() => {
    state.assignments = [];
    state.isLoading = false;
  });

  it("shows a spinner while today's roster loads", () => {
    state.isLoading = true;
    render(<RosterTodayBanner />);
    expect(screen.getByRole("region", { name: "roster.on-duty-today" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("roster.loading");
    expect(screen.queryByText("roster.nobody-today")).not.toBeInTheDocument();
  });

  it("asks for today's roster and names everyone on duty", () => {
    state.assignments = [
      { name: "roster-assignments/1", date: "2026-10-12", user: "users/alice" },
      { name: "roster-assignments/2", date: "2026-10-12", user: "users/bob" },
    ];
    render(<RosterTodayBanner />);

    expect(state.range[0]).toBe(state.range[1]);
    expect(state.range[0]).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(screen.getByRole("region", { name: "roster.on-duty-today" })).toBeInTheDocument();
    expect(screen.getByText("Alice (@alice)")).toBeInTheDocument();
    // An unresolved member still shows by username.
    expect(screen.getByText("@bob")).toBeInTheDocument();
  });

  it("says when nobody is on duty", () => {
    render(<RosterTodayBanner />);
    expect(screen.getByText("roster.nobody-today")).toBeInTheDocument();
  });
});

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import RosterWeekSection from "@/components/AppSidebar/RosterWeekSection";

const state = vi.hoisted(() => ({ range: [] as string[] }));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ generalSetting: { weekStartDayOffset: 1 } }) }));
vi.mock("@/hooks/useRosterQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useRosterQueries")>()),
  useRosterAssignments: (startDate: string, endDate: string) => {
    state.range = [startDate, endDate];
    return {
      assignments: [
        { name: "roster-assignments/1", date: "2026-10-12", user: "users/budi" },
        { name: "roster-assignments/2", date: "2026-10-14", user: "users/sari" },
      ],
      isLoading: false,
    };
  },
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUsersByNames: () => ({ data: new Map([["users/budi", { username: "budi", displayName: "Budi Santoso" }]]) }),
}));

describe("<RosterWeekSection>", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 14, 9, 0));
  });
  afterEach(() => vi.useRealTimers());

  it("lists who is on duty each day of the current week, linking each day to the calendar", () => {
    render(
      <MemoryRouter>
        <RosterWeekSection />
      </MemoryRouter>,
    );

    expect(state.range).toEqual(["2026-10-12", "2026-10-18"]);
    expect(screen.getByText("roster.this-week")).toBeInTheDocument();
    const days = screen.getAllByRole("link");
    expect(days).toHaveLength(7);
    expect(days[0]).toHaveTextContent("Budi Santoso (@budi)");
    expect(days[0]).toHaveAttribute("href", "/calendar/2026/10/12");
    expect(days[1]).toHaveTextContent("—");
    expect(days[2]).toHaveTextContent("@sari");
    expect(days[2]).toHaveAttribute("aria-current", "date");
  });
});

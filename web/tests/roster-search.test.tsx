import { fireEvent, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import QuickFindDialog from "@/components/AppSidebar/QuickFindDialog";

const state = vi.hoisted(() => ({ setQuickFindOpen: vi.fn() }));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, params?: Record<string, unknown>) => (params ? `${key} ${JSON.stringify(params)}` : key),
}));
vi.mock("@/contexts/AppSidebarContext", () => ({
  useAppSidebar: () => ({ quickFindOpen: true, setQuickFindOpen: state.setQuickFindOpen }),
}));
vi.mock("@/hooks/useRosterQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/useRosterQueries")>()),
  useRosterAssignments: () => ({
    assignments: [
      { name: "roster-assignments/1", date: "2026-10-12", user: "users/memberjoss" },
      { name: "roster-assignments/2", date: "2026-10-14", user: "users/budi" },
    ],
    isLoading: false,
  }),
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUsersByNames: () => ({
    data: new Map([
      ["users/memberjoss", { username: "memberjoss", displayName: "Jhon" }],
      ["users/budi", { username: "budi", displayName: "Budi Santoso" }],
    ]),
  }),
}));

const renderSearch = () => {
  const router = createMemoryRouter([{ path: "*", element: <QuickFindDialog /> }], { initialEntries: ["/calendar"] });
  render(<RouterProvider router={router} />);
  return router;
};

describe("roster search", () => {
  beforeEach(() => state.setQuickFindOpen.mockClear());

  it("finds the days a member is on duty by full name or username and opens a day", () => {
    const router = renderSearch();
    expect(screen.getByText("roster.search-hint")).toBeInTheDocument();

    fireEvent.change(screen.getByRole("textbox", { name: "roster.search" }), { target: { value: "jhon" } });
    let results = screen.getAllByRole("button");
    expect(results).toHaveLength(1);
    expect(results[0]).toHaveTextContent("Jhon (@memberjoss)");

    fireEvent.change(screen.getByRole("textbox", { name: "roster.search" }), { target: { value: "BUDI" } });
    results = screen.getAllByRole("button");
    expect(results[0]).toHaveTextContent("Budi Santoso (@budi)");
    fireEvent.click(results[0]);
    expect(router.state.location.pathname).toBe("/calendar/2026/10/14");
    expect(state.setQuickFindOpen).toHaveBeenCalledWith(false);
  });

  it("says the roster was not found when no member matches", () => {
    renderSearch();
    fireEvent.change(screen.getByRole("textbox", { name: "roster.search" }), { target: { value: "nobody" } });
    expect(screen.getByText("roster.not-found")).toBeInTheDocument();
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });
});

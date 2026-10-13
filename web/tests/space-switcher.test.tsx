import { fireEvent, render, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SpaceSwitcher from "@/components/AppSidebar/SpaceSwitcher";
import { getCollectionCreator, ROUTES, resolveCollectionRoute } from "@/router/routes";

const state = vi.hoisted(() => ({
  currentUser: { name: "users/steven", username: "steven", displayName: "Steven", avatarUrl: "" } as
    | { name: string; username: string; displayName: string; avatarUrl: string }
    | undefined,
  setMobileOpen: vi.fn(),
}));

vi.mock("@/components/AboutDialog", () => ({ default: () => null }));
vi.mock("@/components/NotesLogo", () => ({ default: () => <span>Notes</span> }));
vi.mock("@/components/UserMenu", () => ({
  default: () => <button type="button">Steven account</button>,
}));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => state.currentUser,
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUser: () => ({ data: { name: "users/alice", username: "alice", displayName: "Alice", avatarUrl: "" } }),
  useNotifications: () => ({ data: [] }),
}));
vi.mock("@/contexts/AppSidebarContext", () => ({ useAppSidebar: () => ({ setMobileOpen: state.setMobileOpen }) }));
vi.mock("@/contexts/SpaceContext", () => ({
  useSpaceContext: () => {
    const location = useLocation();
    const route = resolveCollectionRoute(location.pathname);
    return {
      creatorUsername:
        route.pathname === ROUTES.EXPLORE
          ? undefined
          : (getCollectionCreator(location.search) ?? (route.pathname === ROUTES.HOME ? state.currentUser?.username : undefined)),
    };
  },
}));
vi.mock("@/utils/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/i18n")>()),
  useTranslate: () => (key: string, options?: { source: string }) => (options?.source ? `Back to ${options.source}` : key),
}));

const renderAt = (path = "/", size: "md" | "header" = "md") => {
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <aside>
            <SpaceSwitcher size={size} />
          </aside>
        ),
      },
    ],
    { initialEntries: [path] },
  );
  render(<RouterProvider router={router} />);
  return router;
};

const switcherTrigger = () => within(document.querySelector("aside") as HTMLElement).getAllByRole("button")[0];
const openSwitcher = () => fireEvent.click(switcherTrigger());

describe("SpaceSwitcher", () => {
  beforeEach(() => {
    state.currentUser = { name: "users/steven", username: "steven", displayName: "Steven", avatarUrl: "" };
    state.setMobileOpen.mockClear();
  });

  it("shows the brand and only the account panel on the calendar", () => {
    renderAt("/calendar/2026/09", "header");
    expect(switcherTrigger()).toHaveAccessibleName("common.notes");
    expect(within(switcherTrigger()).getByText("Notes")).toBeInTheDocument();
    openSwitcher();
    expect(screen.queryByRole("navigation", { name: "common.browse" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.home" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.explore" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Back to/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Steven account" })).toBeInTheDocument();
  });

  it("offers no way back to a narrowed calendar, since notes are not browsed per member", () => {
    renderAt("/calendar/2026/09?creator=alice", "header");
    expect(switcherTrigger()).toHaveAccessibleName("common.notes");
    openSwitcher();
    expect(screen.queryByRole("link", { name: /Back to/ })).not.toBeInTheDocument();
  });

  it("keeps the popup aligned to the sidebar rail", () => {
    renderAt("/calendar");
    const trigger = switcherTrigger();
    const sidebar = trigger.closest("aside") as HTMLElement;
    vi.spyOn(sidebar, "getBoundingClientRect").mockReturnValue({ left: 0, right: 223, width: 223 } as DOMRect);
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({ left: 12, right: 143, width: 131 } as DOMRect);
    openSwitcher();
    expect(document.querySelector('[data-slot="popover-content"]')).toHaveStyle({ width: "199px" });
  });
});

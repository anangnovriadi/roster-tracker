import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, render as testingLibraryRender, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AppSidebar, { MobileAppHeader, MobileAppSidebar } from "@/components/AppSidebar";
import { type NoteFilter } from "@/contexts/NoteFilterContext";
import { getCollectionCreator, resolveCollectionRoute } from "@/router/routes";
import { User_Role } from "@/types/proto/api/user_service_pb";

const authState = vi.hoisted(() => ({
  currentUser: { name: "users/test" } as { name: string; username?: string } | undefined,
  noteViews: [] as Array<{ name: string; title: string }>,
  notifications: [] as Array<{ status: number }>,
  guestCreator: undefined as { username: string; displayName: string; avatarUrl: string } | undefined,
}));
const sidebarState = vi.hoisted(() => ({
  mobileOpen: false,
  setMobileOpen: vi.fn(),
  setQuickFindOpen: vi.fn(),
}));
const globalEditorState = vi.hoisted(() => ({
  canOpen: true,
  openEditor: vi.fn(),
}));
const filteredStatsHook = vi.hoisted(() => vi.fn());
const filterState = vi.hoisted(() => ({ filters: [] as NoteFilter[] }));
const tagsSectionHook = vi.hoisted(() => vi.fn());

vi.mock("@/components/AboutDialog", () => ({ default: () => null }));
vi.mock("@/components/AppSidebar/RosterWeekSection", () => ({ default: () => <section aria-label="roster.this-week" /> }));
vi.mock("@/components/NotesLogo", () => ({
  default: ({ size }: { size?: string }) => <span data-logo-size={size}>Notes logo</span>,
}));

vi.mock("@/components/UserMenu", () => ({
  default: () => (
    <button type="button" className="w-full">
      User menu
    </button>
  ),
}));

vi.mock("@/components/StatisticsView", () => ({
  default: () => <div>Calendar</div>,
}));

vi.mock("@/components/AppSidebar/TagsSection", () => ({
  default: (props: unknown) => {
    tagsSectionHook(props);
    return <div>Tags</div>;
  },
}));

vi.mock("@/contexts/AppSidebarContext", () => ({
  useAppSidebar: () => ({
    attachmentSection: "all",
    setAttachmentSection: vi.fn(),
    inboxFilter: "all",
    setInboxFilter: vi.fn(),
    noteDetail: undefined,
    setNoteDetail: vi.fn(),
    mobileOpen: sidebarState.mobileOpen,
    setMobileOpen: sidebarState.setMobileOpen,
    quickFindOpen: false,
    setQuickFindOpen: sidebarState.setQuickFindOpen,
  }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ isInitialized: true }),
}));

vi.mock("@/contexts/GlobalNoteEditorContext", () => ({
  useGlobalNoteEditor: () => globalEditorState,
}));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ isInitialized: true }),
}));

vi.mock("@/contexts/NoteFilterContext", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/contexts/NoteFilterContext")>()),
  useNoteFilterContext: () => ({
    filters: filterState.filters,
    noteView: undefined,
    setNoteView: vi.fn(),
  }),
}));

vi.mock("@/contexts/SpaceContext", () => ({
  useSpaceContext: () => {
    const location = useLocation();
    const { isCollection } = resolveCollectionRoute(location.pathname);
    return { creatorUsername: isCollection ? getCollectionCreator(location.search) : undefined, noteFilter: undefined };
  },
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => authState.currentUser,
}));

vi.mock("@/hooks/useFilteredNoteStats", () => ({
  useFilteredNoteStats: (options: unknown) => {
    filteredStatsHook(options);
    return { statistics: { activityStats: {}, timeBasis: "create_time" }, tags: {} };
  },
}));

vi.mock("@/hooks/useAttachmentLibrary", () => ({
  useAttachmentLibraryStats: () => ({ stats: { media: 0, documents: 0, audio: 0 } }),
}));

vi.mock("@/hooks/useMediaQuery", () => ({
  default: () => true,
}));

vi.mock("@/hooks/useUserQueries", () => ({
  userKeys: {
    noteViews: (parent?: string) => ["users", "noteViews", parent],
  },
  useNoteViews: () => ({ data: authState.noteViews }),
  useNotifications: () => ({ data: authState.notifications }),
  useUser: () => ({ data: authState.guestCreator }),
}));

vi.mock("@/i18n", () => ({
  default: { language: "en" },
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const render = (ui: Parameters<typeof testingLibraryRender>[0]) =>
  testingLibraryRender(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>,
  );

const expectCollapsedNavPill = (pill: HTMLElement, label: string) => {
  expect(pill).toHaveClass("h-7", "rounded-md", "px-1.5");
  expect(pill.firstElementChild).toHaveClass("size-4");
  const labelTrack = pill.querySelector('span.grid[aria-hidden="true"]');
  expect(labelTrack).toHaveClass("grid-cols-[0fr]", "ps-0");
  expect(labelTrack).not.toHaveClass("@min-[230px]:grid-cols-[1fr]");
  expect(labelTrack).toHaveTextContent(label);
};

const expectExpandedNavPill = (pill: HTMLElement, label: string) => {
  expect(pill).toHaveClass("h-7", "rounded-md", "px-1.5");
  expect(pill.firstElementChild).toHaveClass("size-4");
  const labelTrack = pill.querySelector("span.grid");
  // The label opens only where the row can afford it; the control's aria-label names it regardless.
  expect(labelTrack).toHaveClass("@min-[230px]:grid-cols-[1fr]", "@min-[230px]:ps-1.5");
  expect(labelTrack).toHaveAttribute("aria-hidden", "true");
  expect(pill.querySelector("[data-sidebar-label]")).toHaveTextContent(label);
};

const expectActiveNavPill = (pill: HTMLElement, label: string) => {
  expectExpandedNavPill(pill, label);
  expect(pill).toHaveAttribute("aria-current", "page");
};

describe("App sidebar logo", () => {
  beforeEach(() => {
    authState.currentUser = { name: "users/test" };
    authState.noteViews = [];
    authState.notifications = [];
    authState.guestCreator = undefined;
    sidebarState.mobileOpen = false;
    sidebarState.setMobileOpen.mockClear();
    sidebarState.setQuickFindOpen.mockClear();
    globalEditorState.canOpen = true;
    globalEditorState.openEditor.mockClear();
    filteredStatsHook.mockClear();
    filterState.filters = [];
    tagsSectionHook.mockClear();
  });

  it("preserves creator and filters, excluding view state, when moving between collection pages", () => {
    render(
      <MemoryRouter initialEntries={["/calendar?creator=alice&filter=tagSearch%3Awork&note=notes/a"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    const search = "?filter=tagSearch%3Awork&creator=alice";
    expect(screen.queryByRole("link", { name: "common.timeline" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "common.calendar" })).toHaveAttribute("href", `/calendar${search}`);
    expect(screen.getByRole("link", { name: "common.attachments" })).toHaveAttribute("href", `/attachments${search}`);
  });

  it("shows the context switcher without a compose button", () => {
    authState.currentUser = { name: "users/test", role: User_Role.ADMIN } as never;
    render(
      <MemoryRouter initialEntries={["/attachments"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const switcher = screen.getByRole("button", { name: "common.notes" });
    const header = switcher.closest("[data-sidebar-header]");
    const primaryNavigation = screen.getByRole("navigation", { name: "Primary" });
    const createMember = within(primaryNavigation).getByRole("button", { name: "setting.member.create-member" });

    expect(header).toHaveClass("h-13", "px-3");
    expect(switcher).toHaveTextContent("Notes logo");
    expect(switcher).toHaveClass("min-w-0", "h-9", "gap-2", "px-2");
    expect(switcher).not.toHaveClass("px-1");
    expect(switcher.firstElementChild).not.toHaveClass("flex-1");
    expect(within(switcher).getByText("Notes logo")).toHaveAttribute("data-logo-size", "header");
    expect(switcher.querySelector(".lucide-chevrons-up-down")).not.toBeNull();
    expect(switcher.querySelector(".lucide-chevron-down")).toBeNull();
    expect(screen.queryByRole("button", { name: "editor.new-note" })).not.toBeInTheDocument();
    expect(header).not.toContainElement(createMember);
    expect(createMember).toHaveClass("ms-auto", "h-7", "px-1.5");
    expect(createMember.querySelector(".lucide-plus")).toHaveClass("size-4");

    // The Calendar destination is a nav pill; the statistics calendar stays off this route.
    expect(within(primaryNavigation).getByRole("link", { name: "common.calendar" })).toHaveAttribute("href", "/calendar");
    expect(screen.queryByText("Calendar")).not.toBeInTheDocument();

    // The plus opens the same Create Member dialog as Settings → Member.
    fireEvent.click(createMember);
    expect(sidebarState.setMobileOpen).toHaveBeenCalledWith(false);
    expect(screen.getByRole("dialog")).toHaveTextContent("setting.member.create-member");
  });

  it.each([
    "/archived",
    "/inbox",
    "/setting",
    "/views",
    "/notes/123",
    "/notes/shares/token",
    "/404",
  ])("shows the brand in the switcher on global page %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const brand = screen.getByRole("button", { name: "common.notes" });
    expect(brand).toHaveClass("h-9", "gap-2", "px-2");
    expect(within(brand).getByText("Notes logo")).toHaveAttribute("data-logo-size", "header");
  });

  it("keeps Archived statistics and tag UI state unscoped", () => {
    render(
      <MemoryRouter initialEntries={["/archived"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "archived", filter: undefined }));
    expect(tagsSectionHook).toHaveBeenCalledWith(expect.objectContaining({ scope: "archived" }));
  });

  it("shows signed-in account actions in the top switcher", () => {
    render(
      <MemoryRouter initialEntries={["/inbox"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("link", { name: "common.sign-in-to-notes" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.notes" }));
    expect(screen.getByRole("button", { name: "User menu" })).toBeInTheDocument();
  });

  it("keeps Attachments active after route normalization", () => {
    render(
      <MemoryRouter initialEntries={["/Attachments/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "common.attachments" })).toHaveAttribute("aria-current", "page");
  });

  it("hides Compose when the composer reports it is not available", () => {
    globalEditorState.canOpen = false;
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.queryByRole("button", { name: "editor.new-note" })).not.toBeInTheDocument();
    // Creating members is an admin action.
    expect(screen.queryByRole("button", { name: "setting.member.create-member" })).not.toBeInTheDocument();
  });

  it("shows the compact public navigation for a guest", () => {
    authState.currentUser = undefined;
    globalEditorState.canOpen = false;
    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "common.notes" })).toHaveTextContent("Notes logo");
    const primaryNavigation = screen.getByRole("navigation", { name: "Primary" });
    expect(primaryNavigation).toHaveClass("h-7", "items-center", "gap-1", "px-3");
    expect(primaryNavigation).not.toHaveClass("flex-col");
    const navigation = within(primaryNavigation);
    expect(navigation.queryByRole("button", { name: "setting.member.create-member" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.timeline" })).not.toBeInTheDocument();
    const calendar = navigation.getByRole("link", { name: "common.calendar" });
    expect(calendar).toHaveAttribute("href", "/calendar");
    expectCollapsedNavPill(calendar, "common.calendar");
    expect(navigation.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.attachments" })).not.toBeInTheDocument();
    const signIn = screen.getByRole("link", { name: "common.sign-in-to-notes" });
    expect(signIn).toHaveClass("w-full", "px-5");
    expect(signIn).not.toHaveClass("rounded-md");
    expect(signIn.closest("footer")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "editor.new-note" })).not.toBeInTheDocument();
  });

  it("keeps the brand in the header even when the calendar is narrowed to one member", () => {
    authState.currentUser = undefined;
    authState.guestCreator = { username: "steven", displayName: "Steven", avatarUrl: "/avatar.png" };

    render(
      <MemoryRouter initialEntries={["/calendar?creator=steven"]}>
        <AppSidebar />
        <MobileAppHeader />
      </MemoryRouter>,
    );

    const brands = screen.getAllByRole("button", { name: "common.notes" });
    expect(brands).toHaveLength(2);
    for (const brand of brands) expect(brand).toHaveTextContent("Notes logo");
    expect(screen.queryByRole("button", { name: "Steven" })).not.toBeInTheDocument();
  });

  it("shows the signed-in member's full name and username at the foot of the sidebar", () => {
    authState.currentUser = { name: "users/budi", username: "budi", displayName: "Budi Santoso" } as never;
    render(
      <MemoryRouter initialEntries={["/calendar"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const footer = screen.getByRole("contentinfo", { name: "setting.my-account.label" });
    expect(footer).toHaveTextContent("Budi Santoso");
    expect(footer).toHaveTextContent("@budi");
    expect(footer.previousElementSibling).not.toBeNull();
    expect(footer.nextElementSibling).toBeNull();
  });

  it.each(["/403", "/404", "/unknown"])("uses the common sidebar without inheriting collection content on %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(screen.queryByText("Calendar")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "common.statistics" })).not.toBeInTheDocument();
    expect(screen.queryByText("common.views")).not.toBeInTheDocument();
    expect(screen.queryByText("Tags")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.notes" })).toBeInTheDocument();
    const navigation = within(screen.getByRole("navigation", { name: "Primary" }));
    expect(navigation.queryByRole("link", { name: "common.timeline" })).not.toBeInTheDocument();
    const attachments = navigation.getByRole("link", { name: "common.attachments" });
    expect(attachments).toHaveAttribute("href", "/attachments");
    expectCollapsedNavPill(attachments, "common.attachments");
    expect(screen.queryByRole("link", { name: "common.inbox" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.sign-in-to-notes" })).not.toBeInTheDocument();
  });

  it("uses a visitor sidebar for a guest on a route without contextual content", () => {
    authState.currentUser = undefined;
    render(
      <MemoryRouter initialEntries={["/404"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const navigation = within(screen.getByRole("navigation", { name: "Primary" }));
    expect(navigation.queryByRole("link", { name: "common.timeline" })).not.toBeInTheDocument();
    expectCollapsedNavPill(navigation.getByRole("link", { name: "common.calendar" }), "common.calendar");
    expect(screen.queryByRole("link", { name: "common.attachments" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.inbox" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "common.sign-in-to-notes" }).closest("footer")).not.toBeNull();
  });

  it.each([["/calendar/2026/09", "common.calendar"]])("marks the reading view active for a guest on %s", (path, label) => {
    authState.currentUser = undefined;
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const navigation = within(screen.getByRole("navigation", { name: "Primary" }));
    expectActiveNavPill(navigation.getByRole("link", { name: label }), label);
    expect(screen.queryByText("Calendar")).not.toBeInTheDocument();
  });

  it("shows Calendar and Attachments without Timeline or display settings", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("region", { name: "common.statistics" })).toBeInTheDocument();
    expect(screen.queryByText("common.views")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.timeline" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "note.view-options" })).not.toBeInTheDocument();
    expectCollapsedNavPill(screen.getByRole("link", { name: "common.calendar" }), "common.calendar");
    expectCollapsedNavPill(screen.getByRole("link", { name: "common.attachments" }), "common.attachments");
  });

  it.each(["/", "/explore", "/calendar", "/attachments"])("never shows Timeline display settings: %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("button", { name: "note.view-options" })).not.toBeInTheDocument();
  });

  it("keeps the collection navigation on Attachments", () => {
    render(
      <MemoryRouter initialEntries={["/attachments"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("link", { name: "common.sign-in-to-notes" })).not.toBeInTheDocument();
    expectActiveNavPill(screen.getByRole("link", { name: "common.attachments" }), "common.attachments");
  });

  it.each([
    ["/attachments", "common.attachments"],
    ["/setting", "common.menu"],
  ])("labels the sidebar content section on %s", (path, label) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: label, level: 2 })).toBeInTheDocument();
  });

  it("keeps the mobile header limited to navigation and context", () => {
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <MobileAppHeader />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "Open Navigation" })).toHaveAttribute("data-mobile-navigation-trigger");
    const mobileBrand = screen.getByRole("button", { name: "common.notes" });
    expect(mobileBrand).toHaveClass("h-9", "gap-1.5", "px-1");
    expect(screen.queryByRole("button", { name: "setting.member.create-member" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editor.new-note" })).not.toBeInTheDocument();
  });

  it("offers Create Member to admins but no Compose in the mobile navigation drawer", () => {
    sidebarState.mobileOpen = true;
    authState.currentUser = { name: "users/test", role: User_Role.ADMIN } as never;
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <MobileAppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "setting.member.create-member" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editor.new-note" })).not.toBeInTheDocument();
  });

  it("keeps an accessible Close control in the mobile navigation drawer", () => {
    sidebarState.mobileOpen = true;
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <MobileAppSidebar />
      </MemoryRouter>,
    );

    const dialog = screen.getByRole("dialog");
    const close = screen.getByRole("button", { name: "Close" });
    expect(dialog).toHaveClass("[&>[data-slot=sheet-close]]:sr-only");
    expect(close).toHaveAttribute("data-slot", "sheet-close");
    fireEvent.click(close);
    expect(sidebarState.setMobileOpen.mock.calls.at(-1)?.[0]).toBe(false);
  });
});

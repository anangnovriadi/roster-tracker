import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import UserMenu from "@/components/UserMenu";
import { loadLocale } from "@/utils/i18n";
import { loadTheme } from "@/utils/theme";

const mocks = vi.hoisted(() => ({
  navigateTo: vi.fn(),
  setMobileOpen: vi.fn(),
  onClose: vi.fn(),
  logout: vi.fn(),
  updateGeneralSetting: vi.fn(),
  refetchSettings: vi.fn(),
  currentUser: { name: "users/steven", username: "steven", displayName: "Steven" } as
    | { name: string; username: string; displayName: string }
    | undefined,
  notifications: [] as Array<{ status: number }>,
}));

vi.mock("@/contexts/AppSidebarContext", () => ({ useAppSidebar: () => ({ setMobileOpen: mocks.setMobileOpen }) }));
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ userGeneralSetting: undefined, refetchSettings: mocks.refetchSettings, logout: mocks.logout }),
}));
vi.mock("@/hooks/useLiveNoteRefresh", () => ({ useSSEConnectionStatus: () => "connected" }));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => mocks.currentUser,
}));
vi.mock("@/hooks/useNavigateTo", () => ({ default: () => mocks.navigateTo }));
vi.mock("@/hooks/useUserQueries", () => ({
  useNotifications: () => ({ data: mocks.notifications }),
  useUpdateUserGeneralSetting: () => ({ mutate: mocks.updateGeneralSetting }),
}));
vi.mock("@/utils/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/i18n")>()),
  loadLocale: vi.fn((await importOriginal<typeof import("@/utils/i18n")>()).loadLocale),
  useTranslate: () => (key: string) => key,
}));
vi.mock("@/utils/theme", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/theme")>()),
  loadTheme: vi.fn((await importOriginal<typeof import("@/utils/theme")>()).loadTheme),
}));

const renderMenu = (path = "/") =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <UserMenu onClose={mocks.onClose} />
    </MemoryRouter>,
  );

describe("User menu panel", () => {
  beforeEach(() => {
    mocks.currentUser = { name: "users/steven", username: "steven", displayName: "Steven" };
    localStorage.clear();
    localStorage.setItem("notes-locale", "en");
    localStorage.setItem("notes-theme", "system");
    vi.mocked(loadTheme).mockClear();
    vi.mocked(loadLocale).mockClear();
    mocks.navigateTo.mockReset();
    mocks.setMobileOpen.mockReset();
    mocks.onClose.mockReset();
    mocks.logout.mockReset();
    mocks.updateGeneralSetting.mockReset();
    mocks.refetchSettings.mockReset();
    mocks.notifications = [];
  });

  it("signs out to the public calendar instead of the sign-in page", async () => {
    const originalLocation = window.location;
    const replace = vi.fn();
    Object.defineProperty(window, "location", { configurable: true, value: { ...originalLocation, replace } });
    try {
      renderMenu();
      fireEvent.click(screen.getByRole("button", { name: "common.sign-out" }));
      await waitFor(() => expect(replace).toHaveBeenCalledWith("/calendar"));
      expect(mocks.logout).toHaveBeenCalledOnce();
    } finally {
      Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
    }
  });

  it("shows only Settings and Sign out for a member, without an account row or Inbox", () => {
    renderMenu("/setting");
    expect(screen.queryByText("Steven")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /common.inbox/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.archived" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.sign-out" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.settings" }));
    expect(mocks.navigateTo).toHaveBeenCalledWith("/setting");
  });

  it("offers guests preferences without account pages, About, or a second sign-in", () => {
    mocks.currentUser = undefined;
    renderMenu("/explore");
    expect(screen.getByRole("button", { name: /setting.preference.theme/ })).toBeInTheDocument();
    for (const name of ["common.inbox", "common.archived", "common.settings", "common.sign-out", "common.sign-in", "common.about"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });

  it("shows current preferences on rows that open their choices as submenus", async () => {
    renderMenu();
    fireEvent.click(screen.getByRole("button", { name: /setting.preference.theme.*Sync with system/ }));
    const themes = await screen.findByRole("menu");
    expect(within(themes).getByRole("menuitemradio", { name: "Sync with system" })).toHaveAttribute("aria-checked", "true");
    expect(
      within(themes)
        .getAllByRole("menuitemradio")
        .map((item) => item.textContent),
    ).toEqual(["Sync with system", "Light", "Dark"]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it.each([
    {
      row: /setting.preference.theme/,
      choice: "Dark",
      load: loadTheme,
      value: "default-dark",
      field: "theme",
      label: /setting.preference.theme.*Dark/,
    },
    {
      row: /common.language/,
      choice: "Malaysia",
      load: loadLocale,
      value: "ms",
      field: "locale",
      label: /common.language.*Malaysia/,
    },
  ] as const)("applies and saves $field from its submenu", async ({ row, choice, load, value, field, label }) => {
    renderMenu();
    fireEvent.click(screen.getByRole("button", { name: row }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: choice }));
    expect(load).toHaveBeenCalledWith(value);
    expect(mocks.updateGeneralSetting).toHaveBeenCalledWith(
      { generalSetting: { [field]: value }, updateMask: [field] },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
    expect(mocks.onClose).not.toHaveBeenCalled();
  });

  it.each([
    {
      row: /setting.preference.theme/,
      choice: "Dark",
      storageKey: "notes-theme",
      value: "default-dark",
      label: /setting.preference.theme.*Dark/,
    },
    { row: /common.language/, choice: "Malaysia", storageKey: "notes-locale", value: "ms", label: /common.language.*Malaysia/ },
  ] as const)("persists a guest's choice of $storageKey locally without updating an account", async ({
    row,
    choice,
    storageKey,
    value,
    label,
  }) => {
    mocks.currentUser = undefined;
    const menu = renderMenu("/explore");
    fireEvent.click(screen.getByRole("button", { name: row }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: choice }));
    expect(localStorage.getItem(storageKey)).toBe(value);
    expect(mocks.updateGeneralSetting).not.toHaveBeenCalled();
    expect(mocks.refetchSettings).not.toHaveBeenCalled();
    menu.unmount();
    renderMenu("/explore");
    expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
  });

  it.each([true, false])("puts About just above Sign out and hands it to the host to open (signed in: %s)", (signedIn) => {
    if (!signedIn) mocks.currentUser = undefined;
    const onOpenAbout = vi.fn();
    render(
      <MemoryRouter>
        <UserMenu onClose={mocks.onClose} onOpenAbout={onOpenAbout} />
      </MemoryRouter>,
    );
    const about = screen.getByRole("button", { name: "common.about" });
    if (signedIn) {
      expect(about.previousElementSibling).toBe(screen.getByRole("button", { name: "common.settings" }));
      expect(about.nextElementSibling).toBe(screen.getByRole("button", { name: "common.sign-out" }));
    } else {
      expect(about.nextElementSibling).toBeNull();
    }
    fireEvent.click(about);
    expect(mocks.onClose).toHaveBeenCalled();
    expect(onOpenAbout).toHaveBeenCalledOnce();
  });
});

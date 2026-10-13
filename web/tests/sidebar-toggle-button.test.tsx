import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import SidebarToggleButton from "@/components/AppSidebar/SidebarToggleButton";

const state = vi.hoisted(() => ({ collapsed: false, setCollapsed: vi.fn() }));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/contexts/AppSidebarContext", () => ({
  useAppSidebar: () => ({ desktopCollapsed: state.collapsed, setDesktopCollapsed: state.setCollapsed }),
}));

describe("<SidebarToggleButton>", () => {
  it.each([
    [false, "common.collapse-sidebar", true],
    [true, "common.expand-sidebar", false],
  ])("toggles the sidebar with the same icon when collapsed=%s", (collapsed, label, next) => {
    state.collapsed = collapsed;
    state.setCollapsed.mockClear();
    render(<SidebarToggleButton />);

    const button = screen.getByRole("button", { name: label });
    expect(button.querySelector(".lucide-panel-left")).not.toBeNull();
    expect(button).toHaveAttribute("aria-expanded", String(!collapsed));
    fireEvent.click(button);
    expect(state.setCollapsed).toHaveBeenCalledWith(next);
  });
});

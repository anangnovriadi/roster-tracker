import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import MyAccountSection from "@/components/Settings/MyAccountSection";
import { User_Role } from "@/types/proto/api/user_service_pb";

const state = vi.hoisted(() => ({ role: 0 }));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/connect", () => ({ userServiceClient: {} }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ logout: vi.fn() }) }));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => ({ name: "users/someone", username: "someone", displayName: "Someone", role: state.role, avatarUrl: "" }),
}));
vi.mock("@/hooks/useNavigateTo", () => ({ default: () => vi.fn() }));
vi.mock("@/components/Settings/UsageSection", () => ({ default: () => null }));
vi.mock("@/components/Settings/UserStatsSection", () => ({ default: () => null }));
vi.mock("@/components/ChangeMemberPasswordDialog", () => ({ default: () => null }));
vi.mock("@/components/UpdateAccountDialog", () => ({ default: () => null }));
vi.mock("@/components/ConfirmDialog", () => ({ default: () => null }));

describe("<MyAccountSection>", () => {
  it("hides the danger area from the one admin account", () => {
    state.role = User_Role.ADMIN;
    render(<MyAccountSection />);
    expect(screen.queryByText("setting.account.danger-area")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /setting.account.delete-account/ })).not.toBeInTheDocument();
  });

  it("lets a member delete their own account", () => {
    state.role = User_Role.USER;
    render(<MyAccountSection />);
    expect(screen.getByText("setting.account.danger-area")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /setting.account.delete-account/ })).toBeInTheDocument();
  });
});

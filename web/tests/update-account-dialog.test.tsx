import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import UpdateAccountDialog from "@/components/UpdateAccountDialog";

const state = vi.hoisted(() => ({ updateUser: vi.fn() }));

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("react-hot-toast", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ setCurrentUser: vi.fn() }) }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ generalSetting: {} }) }));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => ({ name: "users/admin", username: "admin", displayName: "", email: "", avatarUrl: "" }),
}));
vi.mock("@/hooks/useUserQueries", () => ({ useUpdateUser: () => ({ mutateAsync: state.updateUser }) }));

const field = (label: string) => screen.getByLabelText(label, { exact: false });

describe("<UpdateAccountDialog>", () => {
  it("asks for full name, username, and email, with no nickname notes or description", async () => {
    render(<UpdateAccountDialog open onOpenChange={vi.fn()} />);

    for (const label of ["setting.member.full-name", "common.username", "common.email"]) {
      expect(field(label)).toHaveAttribute("aria-required", "true");
    }
    expect(screen.queryByText(/nickname/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText("common.description")).not.toBeInTheDocument();

    fireEvent.change(field("common.username"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    expect(await screen.findByText("setting.member.full-name-required")).toBeInTheDocument();
    expect(screen.getByText("setting.account.username-required")).toBeInTheDocument();
    expect(screen.getByText("setting.member.email-required")).toBeInTheDocument();
    expect(state.updateUser).not.toHaveBeenCalled();

    fireEvent.change(field("setting.member.full-name"), { target: { value: "Administrator" } });
    fireEvent.change(field("common.username"), { target: { value: "admin" } });
    fireEvent.change(field("common.email"), { target: { value: "admin@mail.com" } });
    state.updateUser.mockResolvedValue({});
    fireEvent.click(screen.getByRole("button", { name: "common.save" }));
    await waitFor(() => expect(state.updateUser).toHaveBeenCalled());
    expect(state.updateUser.mock.calls[0][0].updateMask).toEqual(["display_name", "email"]);
  });
});

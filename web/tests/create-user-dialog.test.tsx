import { Code, ConnectError } from "@connectrpc/connect";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CreateUserDialog, { buildUsername } from "@/components/CreateUserDialog";
import { User_Role } from "@/types/proto/api/user_service_pb";

const clients = vi.hoisted(() => ({ createUser: vi.fn(), updateUser: vi.fn() }));

vi.mock("@/connect", () => ({ userServiceClient: clients }));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("react-hot-toast", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const renderDialog = (onSuccess = vi.fn()) => {
  render(<CreateUserDialog open onOpenChange={vi.fn()} onSuccess={onSuccess} />);
  return onSuccess;
};

// Required labels end with a decorative "*", so match the label's text loosely.
const field = (label: string) => screen.getByLabelText(label, { exact: false });
const type = (label: string, value: string) => fireEvent.change(field(label), { target: { value } });

describe("buildUsername", () => {
  it("derives a valid username from the full name and a suffix", () => {
    expect(buildUsername("Siti Nur'aini", "x7k2")).toBe("siti-nuraini-x7k2");
    expect(buildUsername("  José  Ángel  ", "ab12")).toBe("jose-angel-ab12");
    expect(buildUsername("", "ab12")).toBe("user-ab12");
    expect(buildUsername("a".repeat(80), "ab12")).toHaveLength(36);
  });
});

describe("<CreateUserDialog>", () => {
  beforeEach(() => {
    clients.createUser.mockReset();
  });

  it("marks full name, email, and password as required", () => {
    renderDialog();
    for (const label of ["setting.member.full-name", "common.email", "common.password"]) {
      expect(field(label)).toHaveAttribute("aria-required", "true");
      const labelElement = document.querySelector(`label[for="${field(label).id}"]`)!;
      expect(labelElement).toHaveTextContent(`${label}*`);
      // The mark shares the label text's span, so the label's gap does not separate them.
      expect(labelElement.querySelector(".text-destructive")?.parentElement?.parentElement).toBe(labelElement);
    }
    expect(document.querySelector('label[for="username"]')).not.toHaveTextContent("*");
  });

  it("requires full name, email, and password, and checks the email format", async () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    expect(await screen.findByText("setting.member.full-name-required")).toBeInTheDocument();
    expect(screen.getByText("setting.member.email-required")).toBeInTheDocument();
    expect(screen.getByText("setting.member.password-required")).toBeInTheDocument();

    type("common.email", "not-an-email");
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    expect(await screen.findByText("setting.member.email-invalid")).toBeInTheDocument();
    expect(clients.createUser).not.toHaveBeenCalled();
  });

  it("generates a read-only username and always creates a member, with no role choice", async () => {
    clients.createUser.mockResolvedValue({});
    const onSuccess = renderDialog();
    type("setting.member.full-name", "Budi Santoso");
    type("common.email", "budi@example.com");
    type("common.password", "secret123");

    const username = field("common.username");
    expect(username).toHaveAttribute("readonly");
    expect((username as HTMLInputElement).value).toMatch(/^budi-santoso-[a-z0-9]{4}$/);
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    const { user } = clients.createUser.mock.calls[0][0];
    expect(user).toMatchObject({ displayName: "Budi Santoso", email: "budi@example.com", password: "secret123", role: User_Role.USER });
    expect(user.username).toBe((username as HTMLInputElement).value);
  });

  it("shows when the email already belongs to another member", async () => {
    clients.createUser.mockRejectedValue(new ConnectError("email is already in use", Code.AlreadyExists));
    renderDialog();
    type("setting.member.full-name", "Budi");
    type("common.email", "taken@example.com");
    type("common.password", "secret123");
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    expect(await screen.findByText("setting.member.email-taken")).toBeInTheDocument();
  });

  it("retries with a fresh suffix when the generated username is taken", async () => {
    clients.createUser.mockRejectedValueOnce(new ConnectError("username is already in use", Code.AlreadyExists)).mockResolvedValue({});
    const onSuccess = renderDialog();
    type("setting.member.full-name", "Budi");
    type("common.email", "budi@example.com");
    type("common.password", "secret123");
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(clients.createUser).toHaveBeenCalledTimes(2);
  });
});

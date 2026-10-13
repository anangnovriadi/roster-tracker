import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import MemberSection from "@/components/Settings/MemberSection";
import { State } from "@/types/proto/api/common_pb";
import { User_Role } from "@/types/proto/api/user_service_pb";

const users = vi.hoisted(() => [
  { name: "users/joss", username: "memberjoss", displayName: "", role: 3, state: 1, email: "", avatarUrl: "" },
  { name: "users/anang", username: "anang", displayName: "", role: 2, state: 1, email: "", avatarUrl: "" },
  {
    name: "users/budi",
    username: "budi-santoso-x7k2",
    displayName: "Budi Santoso",
    role: 3,
    state: 1,
    email: "budi@example.com",
    avatarUrl: "",
  },
]);

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/connect", () => ({ userServiceClient: {} }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/anang" }) }));
vi.mock("@/hooks/useUserQueries", () => ({
  useListUsers: () => ({ data: users, refetch: vi.fn() }),
  useDeleteUser: () => ({ mutate: vi.fn() }),
}));
vi.mock("@/components/CreateUserDialog", () => ({ default: () => null }));
vi.mock("@/components/ConfirmDialog", () => ({ default: () => null }));

describe("<MemberSection>", () => {
  it("numbers the members, lists the admin first, and shows each email", () => {
    expect(users[1].role).toBe(User_Role.ADMIN);
    expect(users[0].state).toBe(State.NORMAL);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemberSection />
      </QueryClientProvider>,
    );

    const rows = screen.getAllByRole("row").slice(1);
    expect(rows[0].querySelector("td")).toHaveTextContent("1");
    expect(rows[2].querySelector("td")).toHaveTextContent("3");
    expect(rows[0]).toHaveTextContent("@anang");
    expect(rows[0]).toHaveTextContent("common.yourself");
    expect(rows[1]).toHaveTextContent("@memberjoss");
    expect(rows[2]).toHaveTextContent("Budi Santoso");
    expect(rows[2]).toHaveTextContent("@budi-santoso-x7k2");
    expect(screen.queryByText("common.empty-placeholder")).not.toBeInTheDocument();
    expect(rows[2]).toHaveTextContent("budi@example.com");
  });
});

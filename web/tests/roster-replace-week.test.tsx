import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { createElement, type PropsWithChildren } from "react";
import { describe, expect, it, vi } from "vitest";
import { useReplaceRosterDays } from "@/hooks/useRosterQueries";
import type { RosterAssignment } from "@/types/proto/api/roster_service_pb";

const client = vi.hoisted(() => ({ createRosterAssignment: vi.fn(), deleteRosterAssignment: vi.fn(), listRosterAssignments: vi.fn() }));
vi.mock("@/connect", () => ({ rosterServiceClient: client }));

const assignment = (name: string, date: string, user: string) => ({ name, date, user }) as RosterAssignment;

describe("useReplaceRosterDays", () => {
  it("puts the member on every day of the week, replacing whoever was on duty", async () => {
    client.createRosterAssignment.mockResolvedValue({});
    client.deleteRosterAssignment.mockResolvedValue({});
    const queryClient = new QueryClient();
    const wrapper = ({ children }: PropsWithChildren) => createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useReplaceRosterDays(), { wrapper });

    const dates = ["2026-10-11", "2026-10-12", "2026-10-13"];
    await act(() =>
      result.current.mutateAsync({
        dates,
        user: "users/admin",
        existing: [
          assignment("roster-assignments/1", "2026-10-11", "users/jhon"),
          assignment("roster-assignments/2", "2026-10-12", "users/admin"),
          // Outside the week: untouched.
          assignment("roster-assignments/3", "2026-10-20", "users/jhon"),
        ],
      }),
    );

    expect(client.deleteRosterAssignment.mock.calls.map(([request]) => request.name)).toEqual(["roster-assignments/1"]);
    expect(client.createRosterAssignment.mock.calls.map(([request]) => request.rosterAssignment.date)).toEqual([
      "2026-10-11",
      "2026-10-13",
    ]);
    expect(client.createRosterAssignment.mock.calls.every(([request]) => request.rosterAssignment.user === "users/admin")).toBe(true);
  });
});

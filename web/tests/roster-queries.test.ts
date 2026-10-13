import { describe, expect, it } from "vitest";
import { formatRosterMember, groupRosterByDate, rosterWeekDates, toRosterDate } from "@/hooks/useRosterQueries";
import type { RosterAssignment } from "@/types/proto/api/roster_service_pb";

describe("roster helpers", () => {
  it("formats a local day as YYYY-MM-DD", () => {
    expect(toRosterDate(new Date(2026, 0, 5))).toBe("2026-01-05");
    expect(toRosterDate(new Date(2026, 11, 31, 23, 59))).toBe("2026-12-31");
  });

  it("groups assignments by date in order", () => {
    const assignment = (name: string, date: string) => ({ name, date, user: `users/${name}` }) as RosterAssignment;
    const grouped = groupRosterByDate([assignment("a", "2026-10-12"), assignment("b", "2026-10-12"), assignment("c", "2026-10-13")]);
    expect([...grouped.keys()]).toEqual(["2026-10-12", "2026-10-13"]);
    expect(grouped.get("2026-10-12")?.map((item) => item.name)).toEqual(["a", "b"]);
  });

  it("names a member by full name and username", () => {
    expect(formatRosterMember("users/budi", { displayName: "Budi Santoso", username: "budi" })).toBe("Budi Santoso (@budi)");
    expect(formatRosterMember("users/budi", { displayName: "", username: "budi" })).toBe("@budi");
    expect(formatRosterMember("users/budi")).toBe("@budi");
  });

  it("lists the seven days of a date's week from the instance's first weekday", () => {
    // 2026-10-14 is a Wednesday.
    expect(rosterWeekDates("2026-10-14", 0)).toEqual([
      "2026-10-11",
      "2026-10-12",
      "2026-10-13",
      "2026-10-14",
      "2026-10-15",
      "2026-10-16",
      "2026-10-17",
    ]);
    expect(rosterWeekDates("2026-10-14", 1)[0]).toBe("2026-10-12");
    expect(rosterWeekDates("2026-10-11", 1)).toEqual([
      "2026-10-05",
      "2026-10-06",
      "2026-10-07",
      "2026-10-08",
      "2026-10-09",
      "2026-10-10",
      "2026-10-11",
    ]);
    // Weeks cross month and year boundaries.
    expect(rosterWeekDates("2026-12-31", 1)).toEqual([
      "2026-12-28",
      "2026-12-29",
      "2026-12-30",
      "2026-12-31",
      "2027-01-01",
      "2027-01-02",
      "2027-01-03",
    ]);
  });
});

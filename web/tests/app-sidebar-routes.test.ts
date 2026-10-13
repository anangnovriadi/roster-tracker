import { describe, expect, it } from "vitest";
import { getRouteActionPolicy, getSidebarRouteKind } from "@/components/AppSidebar/routes";

describe("sidebar route content", () => {
  it.each([
    ["/", "home"],
    ["/archived", "archived"],
    ["/explore", "explore"],
    ["/Explore/", "explore"],
    ["/ARCHIVED/", "archived"],
    ["/u/steven", "common"],
    ["/U/Steven/", "common"],
    ["/calendar", "calendar"],
    ["/calendar/2026/08", "calendar"],
    ["/Calendar/2026/08/02/", "calendar"],
    ["/attachments", "attachments"],
    ["/Attachments/", "attachments"],
    ["/setting", "settings"],
    ["/Setting/", "settings"],
    ["/notes/abc", "memo"],
    ["/Notes/ABC/", "memo"],
    ["/notes/shares/token", "memo"],
    ["/Notes/Shares/token/", "memo"],
    ["/about", "common"],
    ["/403", "common"],
    ["/404", "common"],
    ["/unknown", "common"],
  ])("maps %s to %s content", (path, kind) => {
    expect(getSidebarRouteKind(path)).toBe(kind);
  });

  it.each(["/calendar", "/calendar/2026/08/02", "/Calendar/2026/08/"])("filters the calendar in place on %s", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({ searchScope: "route-collection" });
  });

  it("sends a search from Attachments to the calendar of the same creator", () => {
    expect(getRouteActionPolicy("/attachments")).toEqual({ searchScope: "route-collection", searchDestination: "/calendar" });
    expect(getRouteActionPolicy("/attachments", "?creator=alice")).toEqual({
      searchScope: "route-collection",
      searchDestination: "/calendar?creator=alice",
    });
  });

  it.each(["/setting", "/notes/abc", "/notes/shares/token", "/403", "/404", "/unknown"])("sends search to the calendar on %s", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "all",
      searchDestination: "/calendar",
    });
  });
});

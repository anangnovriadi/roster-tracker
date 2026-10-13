import { describe, expect, it } from "vitest";
import { buildNoteFilter } from "@/hooks/useNoteFilters";
import { combineCELFilters } from "@/lib/cel-filter";
import { getNoteViewId, isNoteCollectionRoute, isNoteScopeRoute, resolveNoteScope } from "@/lib/note-views";
import { Visibility } from "@/types/proto/api/note_service_pb";

describe("note scopes", () => {
  it("resolves collection and detail routes", () => {
    expect(resolveNoteScope("/archived")).toBe("archived");
    expect(resolveNoteScope("/explore")).toBe("explore");
    expect(resolveNoteScope("/notes/123", { detailFrom: "/archived?filter=tagSearch%3Awork" })).toBe("archived");
    expect(resolveNoteScope("/notes/123", { noteArchived: true })).toBe("archived");
    expect(resolveNoteScope("/settings", { fallback: "explore" })).toBe("explore");
  });

  it("recognizes Home, Archived, and the legacy Explore alias", () => {
    expect(isNoteScopeRoute("/")).toBe(true);
    expect(isNoteScopeRoute("/explore")).toBe(true);
    expect(isNoteScopeRoute("/archived")).toBe(true);
    expect(isNoteScopeRoute("/attachments")).toBe(false);
  });

  it("treats Home and calendar as collection routes where views and filters apply in place", () => {
    expect(isNoteCollectionRoute("/")).toBe(true);
    expect(isNoteCollectionRoute("/archived")).toBe(true);
    expect(isNoteCollectionRoute("/?creator=steven")).toBe(true);
    expect(isNoteCollectionRoute("/u/steven")).toBe(false);
    expect(isNoteCollectionRoute("/calendar")).toBe(true);
    expect(isNoteCollectionRoute("/Calendar/2026/08/02/")).toBe(true);
    expect(isNoteCollectionRoute("/calendars")).toBe(false);
    expect(isNoteScopeRoute("/u/steven")).toBe(false);
    expect(isNoteCollectionRoute("/notes/123")).toBe(false);
    expect(isNoteCollectionRoute("/attachments")).toBe(false);
  });
});

describe("note views", () => {
  it("keeps OR expressions inside the author, Space, view, and facet constraints", () => {
    const query = 'content.contains("plan") || pinned';
    const filter = buildNoteFilter({
      creatorName: "users/steven",
      selectedNoteViewFilter: "has_link || has_code",
      filters: [
        { factor: "celSearch", value: query },
        { factor: "tagSearch", value: "work" },
      ],
      includePinned: true,
    });
    expect(combineCELFilters('space == "spaces/product"', filter)).toBe(
      '(space == "spaces/product") && ((creator == "users/steven") && (has_link || has_code) && (content.contains("plan") || pinned) && (tag in ["work"]))',
    );
  });
  it("extracts the saved view ID", () => {
    expect(getNoteViewId("users/steven/views/work")).toBe("work");
  });

  it("composes a saved task view with search, tags, creator, and visibility", () => {
    expect(
      buildNoteFilter({
        creatorName: "users/steven",
        selectedNoteViewFilter: "has_task_list && has_incomplete_tasks",
        filters: [
          { factor: "contentSearch", value: "plan" },
          { factor: "tagSearch", value: "work" },
        ],
        includePinned: false,
        visibilities: [Visibility.PUBLIC],
      }),
    ).toBe(
      '(creator == "users/steven") && (has_task_list && has_incomplete_tasks) && (content.contains("plan")) && (tag in ["work"]) && (visibility in ["PUBLIC"])',
    );
  });

  it("maps the Space audience without falling back to Private", () => {
    expect(
      buildNoteFilter({
        filters: [],
        includePinned: false,
        visibilities: [Visibility.PUBLIC, Visibility.PROTECTED, Visibility.SPACE],
      }),
    ).toBe('(visibility in ["PUBLIC", "PROTECTED", "SPACE"])');
  });

  it("maps property filter factors to their CEL flags", () => {
    expect(
      buildNoteFilter({
        filters: [
          { factor: "property.hasLink", value: "" },
          { factor: "property.hasTaskList", value: "" },
          { factor: "property.hasCode", value: "" },
          { factor: "property.hasLocation", value: "" },
        ],
        includePinned: false,
      }),
    ).toBe("(has_link) && (has_task_list) && (has_code) && (has_location)");
  });

  it("uses a saved note view filter", () => {
    expect(
      buildNoteFilter({
        filters: [],
        includePinned: false,
        selectedNoteViewFilter: 'tag in ["work"]',
      }),
    ).toBe('(tag in ["work"])');
  });

  it("builds display-time filters from valid local calendar-day boundaries", () => {
    const start = new Date(2026, 7, 2);
    const end = new Date(start);
    end.setDate(end.getDate() + 1);

    expect(
      buildNoteFilter({
        filters: [{ factor: "displayTime", value: "2026-08-02" }],
        includePinned: false,
      }),
    ).toBe(
      `(created_ts >= timestamp(${Math.floor(start.getTime() / 1000)}) && created_ts < timestamp(${Math.floor(end.getTime() / 1000)}))`,
    );
  });

  it("selects on update time when that is the basis", () => {
    const start = new Date(2026, 7, 2);
    const end = new Date(start);
    end.setDate(end.getDate() + 1);

    expect(
      buildNoteFilter({
        filters: [{ factor: "displayTime", value: "2026-08-02" }],
        includePinned: false,
        timeBasis: "update_time",
      }),
    ).toBe(
      `(updated_ts >= timestamp(${Math.floor(start.getTime() / 1000)}) && updated_ts < timestamp(${Math.floor(end.getTime() / 1000)}))`,
    );
  });

  it("ignores invalid display-time filter values", () => {
    expect(
      buildNoteFilter({
        filters: [{ factor: "displayTime", value: "2026-02-30" }],
        includePinned: false,
      }),
    ).toBeUndefined();
  });
});

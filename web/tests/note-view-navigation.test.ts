import { describe, expect, it } from "vitest";
import {
  createNoteNavigationState,
  isNoteDetailPath,
  isNoteResourcePath,
  resolveNoteDetailOrigin,
  resolveNoteParentPage,
  withNoteFilter,
} from "@/components/NoteView/navigation";

describe("note view navigation", () => {
  it("captures the complete collection origin", () => {
    expect(
      resolveNoteParentPage({
        pathname: "/archived",
        search: "?filter=tagSearch%3Awork",
        noteName: "notes/123",
      }),
    ).toBe("/archived?filter=tagSearch%3Awork");
  });

  it("keeps an explicit origin on a canonical note route", () => {
    expect(
      resolveNoteParentPage({
        explicitParentPage: "/explore?filter=contentSearch%3Aroadmap",
        pathname: "/notes/123",
        search: "",
        noteName: "notes/123",
      }),
    ).toBe("/explore?filter=contentSearch%3Aroadmap");
  });

  it("keeps a filtered creator collection origin intact", () => {
    expect(resolveNoteParentPage({ pathname: "/", search: "?creator=alice&filter=tagSearch%3Awork", noteName: "notes/123" })).toBe(
      "/?creator=alice&filter=tagSearch%3Awork",
    );
  });

  it("uses Home for direct and shared resource entries", () => {
    expect(resolveNoteDetailOrigin(undefined)).toBe("/");
    expect(resolveNoteDetailOrigin({ unrelated: true })).toBe("/");
  });

  it("uses the user archive for a direct entry to an archived note", () => {
    expect(resolveNoteDetailOrigin(undefined, { noteArchived: true })).toBe("/archived");
  });

  it("keeps an explicit origin ahead of the archived fallback", () => {
    expect(resolveNoteDetailOrigin(createNoteNavigationState("/?creator=alice"), { noteArchived: true })).toBe("/?creator=alice");
  });

  it("round-trips an explicit origin through router state", () => {
    expect(resolveNoteDetailOrigin(createNoteNavigationState("/explore?filter=tagSearch%3Awork"))).toBe("/explore?filter=tagSearch%3Awork");
  });

  it.each([
    "/notes/123",
    "/notes/123/",
    "/Notes/123",
    "/notes/shares/token",
    "/Notes/Shares/token/",
  ])("does not treat %s as its own origin", (pathname) => {
    expect(isNoteDetailPath(pathname, "notes/123")).toBe(true);
    expect(resolveNoteParentPage({ pathname, search: "", noteName: "notes/123" })).toBe("/");
  });

  it("does not confuse a different note route with the current note detail", () => {
    const pathname = "/notes/1234";
    expect(isNoteDetailPath(pathname, "notes/123")).toBe(false);
    expect(resolveNoteParentPage({ pathname, search: "", noteName: "notes/123" })).toBe(pathname);
  });

  it("rejects extra path segments after a share token", () => {
    expect(isNoteDetailPath("/notes/shares/token/extra", "notes/123")).toBe(false);
  });

  it.each([
    "/notes/123",
    "/Notes/123/",
    "/notes/shares/token",
    "/Notes/Shares/token/",
  ])("recognizes %s as a Note resource route", (pathname) => {
    expect(isNoteResourcePath(pathname)).toBe(true);
  });

  it.each(["/", "/notes", "/notes/123/extra", "/notes/shares/token/extra"])("rejects %s as a Note resource route", (pathname) => {
    expect(isNoteResourcePath(pathname)).toBe(false);
  });

  it("replaces the filter without dropping other origin query parameters", () => {
    expect(withNoteFilter("/?creator=alice&sort=displayTime&filter=old#section", "tagSearch:design")).toBe(
      "/?creator=alice&sort=displayTime&filter=tagSearch%3Adesign",
    );
  });

  it("applies a filter within the originating creator collection", () => {
    expect(withNoteFilter("/?creator=alice", "tagSearch:design")).toBe("/?creator=alice&filter=tagSearch%3Adesign");
  });
});

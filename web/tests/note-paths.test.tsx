import { describe, expect, it } from "vitest";
import { buildNotePath } from "@/router/routes";

describe("note paths", () => {
  it("builds /notes/ URLs from the API's notes/ names", () => {
    expect(buildNotePath("notes/abc123")).toBe("/notes/abc123");
  });
});

import { render } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { LegacyMemoRedirect } from "@/router/LegacyMemoRedirect";
import { buildNotePath } from "@/router/routes";

describe("note paths", () => {
  it("builds /notes/ URLs from the API's memos/ names", () => {
    expect(buildNotePath("memos/abc123")).toBe("/notes/abc123");
  });

  it.each([
    ["/memos/abc123?x=1#memo-comments", "/notes/abc123", "?x=1", "#memo-comments"],
    ["/memos/shares/token", "/notes/shares/token", "", ""],
  ])("redirects %s to /notes/, keeping the query and hash", (from, pathname, search, hash) => {
    const router = createMemoryRouter(
      [
        { path: "memos/*", element: <LegacyMemoRedirect /> },
        { path: "notes/*", element: <div /> },
      ],
      { initialEntries: [from] },
    );
    render(<RouterProvider router={router} />);
    expect(router.state.location).toMatchObject({ pathname, search, hash });
  });
});

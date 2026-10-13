import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { NoteFilterProvider } from "@/contexts/NoteFilterContext";
import { useDateFilterNavigation } from "@/hooks/useDateFilterNavigation";

const Harness = () => {
  const location = useLocation();
  const navigateToDate = useDateFilterNavigation();
  return (
    <>
      <output data-testid="search">{location.search}</output>
      <button type="button" onClick={() => navigateToDate("2026-08-02")}>
        Select date
      </button>
    </>
  );
};

describe("useDateFilterNavigation", () => {
  it("preserves unrelated query parameters while replacing the date filter", async () => {
    render(
      <MemoryRouter initialEntries={["/?creator=steven&sort=displayTime&filter=tagSearch%3Awork"]}>
        <NoteFilterProvider>
          <Harness />
        </NoteFilterProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Select date" }));

    await waitFor(() => {
      const params = new URLSearchParams(screen.getByTestId("search").textContent ?? "");
      expect(params.get("sort")).toBe("displayTime");
      expect(params.get("creator")).toBe("steven");
      expect(params.get("filter")).toContain("tagSearch:work");
      expect(params.get("filter")).toContain("displayTime:2026-08-02");
    });
  });
});

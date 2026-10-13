import { fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import AudienceMenu from "@/components/MemoEditor/Toolbar/AudienceMenu";
import { Visibility } from "@/types/proto/api/memo_service_pb";

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

// Base UI menus reach for layout/pointer APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
  Element.prototype.hasPointerCapture = vi.fn(() => false);
  Element.prototype.setPointerCapture = vi.fn();
  Element.prototype.releasePointerCapture = vi.fn();
});

const openMenu = async () => {
  fireEvent.click(screen.getByRole("button"));
  await screen.findByRole("menu");
};

const visibilityRows = () =>
  screen
    .getAllByRole("menuitem")
    .map((item) => item.textContent)
    .filter((text) => text?.startsWith("memo.visibility."));

beforeEach(() => {
  vi.clearAllMocks();
});

describe("AudienceMenu visibility", () => {
  it("offers Private, Protected, and Public only", async () => {
    render(<AudienceMenu value={Visibility.PRIVATE} onChange={vi.fn()} />);
    await openMenu();

    expect(visibilityRows()).toEqual([
      "memo.visibility.privatememo.visibility.private-description",
      "memo.visibility.protectedmemo.visibility.protected-description",
      "memo.visibility.publicmemo.visibility.public-description",
    ]);
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });

  it("keeps naming a legacy Members audience so it is not silently changed", async () => {
    render(<AudienceMenu value={Visibility.SPACE} onChange={vi.fn()} />);

    expect(screen.getByRole("button")).toHaveTextContent("memo.visibility.space");
    await openMenu();
    expect(screen.getByRole("menuitem", { name: /memo\.visibility\.space/ })).toBeInTheDocument();
  });

  it("reports the picked audience", async () => {
    const onChange = vi.fn();
    render(<AudienceMenu value={Visibility.PRIVATE} onChange={onChange} />);
    await openMenu();

    fireEvent.click(screen.getByRole("menuitem", { name: /memo\.visibility\.public/ }));

    expect(onChange).toHaveBeenCalledWith(Visibility.PUBLIC);
  });
});

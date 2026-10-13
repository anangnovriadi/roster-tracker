import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import MemoActionMenu from "@/components/MemoActionMenu";
import { State } from "@/types/proto/api/common_pb";
import { type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";

const handlers = vi.hoisted(() => ({
  handleEditMemoClick: vi.fn(),
  handleToggleMemoStatusClick: vi.fn(),
  handleCheckAllTaskListItemsClick: vi.fn(),
  handleUncheckAllTaskListItemsClick: vi.fn(),
  handleDeleteMemoClick: vi.fn(),
  confirmDeleteMemo: vi.fn(),
}));

vi.mock("@/components/ConfirmDialog", () => ({
  default: () => null,
}));

vi.mock("@/components/MemoActionMenu/hooks", () => ({
  useMemoActionHandlers: () => handlers,
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const renderMenu = (memo: Memo, props: { readonly?: boolean } = {}) =>
  render(
    <MemoryRouter initialEntries={["/explore"]}>
      <MemoActionMenu memo={memo} {...props} />
    </MemoryRouter>,
  );

const openMenu = async (memo: Memo) => {
  renderMenu(memo);
  fireEvent.click(screen.getByRole("button", { name: "common.more" }));
  await screen.findByRole("menuitem", { name: "common.delete" });
};

const menuItemLabels = () => screen.getAllByRole("menuitem").map((item) => item.textContent);

const memoOf = (fields: Parameters<typeof create<typeof MemoSchema>>[1] = {}) =>
  create(MemoSchema, { name: "memos/1", state: State.NORMAL, ...fields });

describe("MemoActionMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("offers an author Edit and Delete only, without Pin, Open, or Copy", async () => {
    await openMenu(memoOf({ pinned: true }));
    expect(menuItemLabels()).toEqual(["common.edit", "common.delete"]);
    const separators = screen.getAllByRole("separator");
    expect(separators[separators.length - 1].nextElementSibling).toBe(screen.getByRole("menuitem", { name: "common.delete" }));
  });

  it.each([
    [true, "memo.task-actions.check-all", "handleCheckAllTaskListItemsClick"],
    [false, "memo.task-actions.uncheck-all", "handleUncheckAllTaskListItemsClick"],
  ] as const)("shows one task action right after Edit (open tasks: %s)", async (hasIncompleteTasks, label, handler) => {
    await openMenu(memoOf({ property: { hasTaskList: true, hasIncompleteTasks } }));
    expect(menuItemLabels()).toEqual(["common.edit", label, "common.delete"]);
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    expect(handlers[handler]).toHaveBeenCalledOnce();
  });

  it("deletes a comment from the same menu", async () => {
    await openMenu(memoOf({ name: "memos/2", parent: "memos/1" }));
    expect(menuItemLabels()).toEqual(["common.edit", "common.delete"]);
    fireEvent.click(screen.getByRole("menuitem", { name: "common.delete" }));
    expect(handlers.handleDeleteMemoClick).toHaveBeenCalledOnce();
  });

  it("gives a viewer no menu at all", () => {
    renderMenu(memoOf(), { readonly: true });
    expect(screen.queryByRole("button", { name: "common.more" })).not.toBeInTheDocument();
  });

  it("is a quiet compact control that takes the accent fill while open", async () => {
    renderMenu(memoOf());

    const trigger = screen.getByRole("button", { name: "common.more" });
    expect(trigger).toHaveClass("size-6", "rounded-md", "text-muted-foreground/70", "hover:bg-muted/60", "data-popup-open:bg-accent");
    expect(trigger.className).not.toMatch(/ring-/);
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger.querySelector(".lucide-ellipsis-vertical")).toHaveClass("size-4");

    fireEvent.click(trigger);

    await waitFor(() => expect(trigger).toHaveAttribute("aria-expanded", "true"));
    expect(trigger).toHaveAttribute("data-popup-open");
    expect(await screen.findByRole("menuitem", { name: "common.edit" })).toBeInTheDocument();
  });
});

import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NoteActionMenu from "@/components/NoteActionMenu";
import { State } from "@/types/proto/api/common_pb";
import { type Note, NoteSchema } from "@/types/proto/api/note_service_pb";

const handlers = vi.hoisted(() => ({
  handleEditNoteClick: vi.fn(),
  handleToggleNoteStatusClick: vi.fn(),
  handleCheckAllTaskListItemsClick: vi.fn(),
  handleUncheckAllTaskListItemsClick: vi.fn(),
  handleDeleteNoteClick: vi.fn(),
  confirmDeleteNote: vi.fn(),
}));

vi.mock("@/components/ConfirmDialog", () => ({
  default: () => null,
}));

vi.mock("@/components/NoteActionMenu/hooks", () => ({
  useNoteActionHandlers: () => handlers,
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const renderMenu = (note: Note, props: { readonly?: boolean } = {}) =>
  render(
    <MemoryRouter initialEntries={["/explore"]}>
      <NoteActionMenu note={note} {...props} />
    </MemoryRouter>,
  );

const openMenu = async (note: Note) => {
  renderMenu(note);
  fireEvent.click(screen.getByRole("button", { name: "common.more" }));
  await screen.findByRole("menuitem", { name: "common.delete" });
};

const menuItemLabels = () => screen.getAllByRole("menuitem").map((item) => item.textContent);

const noteOf = (fields: Parameters<typeof create<typeof NoteSchema>>[1] = {}) =>
  create(NoteSchema, { name: "notes/1", state: State.NORMAL, ...fields });

describe("NoteActionMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("offers an author Edit and Delete only, without Pin, Open, or Copy", async () => {
    await openMenu(noteOf({ pinned: true }));
    expect(menuItemLabels()).toEqual(["common.edit", "common.delete"]);
    const separators = screen.getAllByRole("separator");
    expect(separators[separators.length - 1].nextElementSibling).toBe(screen.getByRole("menuitem", { name: "common.delete" }));
  });

  it.each([
    [true, "note.task-actions.check-all", "handleCheckAllTaskListItemsClick"],
    [false, "note.task-actions.uncheck-all", "handleUncheckAllTaskListItemsClick"],
  ] as const)("shows one task action right after Edit (open tasks: %s)", async (hasIncompleteTasks, label, handler) => {
    await openMenu(noteOf({ property: { hasTaskList: true, hasIncompleteTasks } }));
    expect(menuItemLabels()).toEqual(["common.edit", label, "common.delete"]);
    fireEvent.click(screen.getByRole("menuitem", { name: label }));
    expect(handlers[handler]).toHaveBeenCalledOnce();
  });

  it("deletes a comment from the same menu", async () => {
    await openMenu(noteOf({ name: "notes/2", parent: "notes/1" }));
    expect(menuItemLabels()).toEqual(["common.edit", "common.delete"]);
    fireEvent.click(screen.getByRole("menuitem", { name: "common.delete" }));
    expect(handlers.handleDeleteNoteClick).toHaveBeenCalledOnce();
  });

  it("gives a viewer no menu at all", () => {
    renderMenu(noteOf(), { readonly: true });
    expect(screen.queryByRole("button", { name: "common.more" })).not.toBeInTheDocument();
  });

  it("is a quiet compact control that takes the accent fill while open", async () => {
    renderMenu(noteOf());

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

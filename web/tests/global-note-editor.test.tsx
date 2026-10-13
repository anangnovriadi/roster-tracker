import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NoteEditorProps } from "@/components/NoteEditor/types";
import { GlobalNoteEditorProvider, useGlobalNoteEditor } from "@/contexts/GlobalNoteEditorContext";

const DIALOG = { name: "editor.new-note" };

vi.mock("@/hooks/useNoteSuggestions", () => ({
  useNoteSuggestions: () => [{ id: "tag:work" }],
}));

const mocks = vi.hoisted(() => ({
  currentUser: { name: "users/test" } as { name: string } | undefined,
  isUserSettingsInitialized: true,
  editorProps: undefined as NoteEditorProps | undefined,
  loadNoteEditor: vi.fn(),
  setMobileOpen: vi.fn(),
  setQuickFindOpen: vi.fn(),
  pathname: "/",
  desktop: true,
}));

vi.mock("react-router-dom", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-router-dom")>()),
  useLocation: () => ({ pathname: mocks.pathname }),
}));

vi.mock("@/components/NoteEditor/loader", () => ({
  loadNoteEditor: mocks.loadNoteEditor,
}));

vi.mock("@/contexts/AppSidebarContext", () => ({
  useAppSidebar: () => ({ setMobileOpen: mocks.setMobileOpen, setQuickFindOpen: mocks.setQuickFindOpen }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ isUserSettingsInitialized: mocks.isUserSettingsInitialized }),
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => mocks.currentUser,
}));

vi.mock("@/hooks/useMediaQuery", () => ({
  default: () => mocks.desktop,
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const MockNoteEditor = (props: NoteEditorProps) => {
  mocks.editorProps = props;
  return (
    <div data-testid="global-editor">
      <button type="button" onClick={() => props.onConfirm?.("notes/created")}>
        Save note
      </button>
      <button type="button" onClick={props.onCancel}>
        Cancel editor
      </button>
      <button type="button" onClick={props.onFocusModeExit}>
        Exit focus mode
      </button>
      <button type="button" onClick={() => props.onSavingChange?.(true)}>
        Start saving
      </button>
    </div>
  );
};

const Trigger = () => {
  const { openEditor } = useGlobalNoteEditor();
  return (
    <button type="button" onClick={openEditor}>
      Open editor
    </button>
  );
};

/** Stands in for the mobile drawer: its Compose button unmounts as the drawer closes. */
const EphemeralTrigger = () => {
  const { openEditor } = useGlobalNoteEditor();
  const [dismissed, setDismissed] = useState(false);
  return (
    <>
      {!dismissed && (
        <button
          type="button"
          data-new-note-trigger
          onClick={() => {
            setDismissed(true);
            openEditor();
          }}
        >
          Open drawer editor
        </button>
      )}
      <button type="button" data-mobile-navigation-trigger>
        Open navigation
      </button>
    </>
  );
};

const renderProvider = (children = <Trigger />) => render(<GlobalNoteEditorProvider>{children}</GlobalNoteEditorProvider>);

/** Renders, focuses the trigger so focus-return has something to restore, and opens. */
const openViaTrigger = async () => {
  renderProvider();
  const trigger = screen.getByRole("button", { name: "Open editor" });
  trigger.focus();
  fireEvent.click(trigger);
  await screen.findByRole("dialog", DIALOG);
  return trigger;
};

const expectClosed = () => waitFor(() => expect(screen.queryByRole("dialog", DIALOG)).not.toBeInTheDocument());

describe("GlobalNoteEditorProvider", () => {
  beforeEach(() => {
    mocks.currentUser = { name: "users/test" };
    mocks.isUserSettingsInitialized = true;
    mocks.editorProps = undefined;
    mocks.loadNoteEditor.mockResolvedValue({ default: MockNoteEditor });
    mocks.setMobileOpen.mockClear();
    mocks.setQuickFindOpen.mockClear();
    mocks.pathname = "/";
    mocks.desktop = true;
  });

  it("allows Home autofocus once after settings finish loading", () => {
    let claimFocus!: () => boolean;
    const Probe = () => {
      claimFocus = useGlobalNoteEditor().claimHomeAutoFocus;
      return null;
    };
    mocks.isUserSettingsInitialized = false;
    const { rerender } = renderProvider(<Probe />);
    mocks.isUserSettingsInitialized = true;
    rerender(
      <GlobalNoteEditorProvider>
        <Probe />
      </GlobalNoteEditorProvider>,
    );

    expect(claimFocus()).toBe(true);
    expect(claimFocus()).toBe(false);
  });

  it.each(["mobile", "other route", "navigation", "existing focus"])("does not autofocus Home after %s", (scenario) => {
    let claimFocus!: () => boolean;
    const Probe = () => {
      claimFocus = useGlobalNoteEditor().claimHomeAutoFocus;
      return <button type="button">Another control</button>;
    };
    if (scenario === "mobile") mocks.desktop = false;
    if (scenario === "other route") mocks.pathname = "/explore";
    const { rerender } = renderProvider(<Probe />);

    if (scenario === "navigation") mocks.pathname = "/inbox";
    if (scenario === "existing focus") screen.getByRole("button", { name: "Another control" }).focus();
    rerender(
      <GlobalNoteEditorProvider>
        <Probe />
      </GlobalNoteEditorProvider>,
    );

    // Returning to the original route or viewport must not rearm focus.
    mocks.pathname = "/";
    mocks.desktop = true;
    rerender(
      <GlobalNoteEditorProvider>
        <Probe />
      </GlobalNoteEditorProvider>,
    );
    expect(claimFocus()).toBe(false);
  });

  it("opens a modal focus-mode editor, closes the sidebar surfaces, and restores focus after Escape", async () => {
    const trigger = await openViaTrigger();
    const dialog = screen.getByRole("dialog", DIALOG);

    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(mocks.setMobileOpen).toHaveBeenCalledWith(false);
    expect(mocks.setQuickFindOpen).toHaveBeenCalledWith(false);
    // `onFocusModeExit` is the hosted marker: the editor mounts in focus mode and
    // exits by dismissing this dialog rather than collapsing inline.
    expect(mocks.editorProps).toMatchObject({
      autoFocus: true,
      cacheKey: "global-note-editor",
      suggestions: [{ id: "tag:work" }],
      onFocusModeExit: expect.any(Function),
    });
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement | null));

    fireEvent.keyDown(dialog, { key: "Escape" });

    await expectClosed();
    expect(trigger).toHaveFocus();
  });

  it.each(["Save note", "Cancel editor", "Exit focus mode", "common.close"])("closes after %s", async (action) => {
    const trigger = await openViaTrigger();

    fireEvent.click(screen.getByRole("button", { name: action }));

    await expectClosed();
    expect(trigger).toHaveFocus();
  });

  it("prevents dialog and focus-mode dismissal while saving but lets save completion force close", async () => {
    await openViaTrigger();
    const dialog = screen.getByRole("dialog", DIALOG);

    fireEvent.click(screen.getByRole("button", { name: "Start saving" }));
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(dialog).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Exit focus mode" }));
    expect(dialog).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "common.close" }));
    expect(dialog).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Save note" }));
    await expectClosed();
  });

  it("makes the screen-reader close control visible when keyboard-focused", async () => {
    await openViaTrigger();
    const close = screen.getByRole("button", { name: "common.close" });

    expect(close).toHaveClass("sr-only", "focus-visible:not-sr-only");
    close.focus();
    expect(close).toHaveFocus();
  });

  it("returns focus to mobile navigation when the drawer's Compose trigger unmounts", async () => {
    renderProvider(<EphemeralTrigger />);

    fireEvent.click(screen.getByRole("button", { name: "Open drawer editor" }));
    await screen.findByRole("dialog", DIALOG);
    fireEvent.click(screen.getByRole("button", { name: "Cancel editor" }));

    await expectClosed();
    expect(screen.getByRole("button", { name: "Open navigation" })).toHaveFocus();
  });

  it("does not open or load the editor for a guest", () => {
    mocks.currentUser = undefined;
    renderProvider();

    fireEvent.click(screen.getByRole("button", { name: "Open editor" }));

    expect(mocks.loadNoteEditor).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("invalidates a pending open request across sign-out and same-user sign-in", async () => {
    let resolveEditorLoad!: (module: { default: typeof MockNoteEditor }) => void;
    mocks.loadNoteEditor.mockReturnValue(
      new Promise((resolve) => {
        resolveEditorLoad = resolve;
      }),
    );
    const view = renderProvider();

    fireEvent.click(screen.getByRole("button", { name: "Open editor" }));
    expect(mocks.loadNoteEditor).toHaveBeenCalledOnce();

    mocks.currentUser = undefined;
    view.rerender(
      <GlobalNoteEditorProvider>
        <Trigger />
      </GlobalNoteEditorProvider>,
    );
    mocks.currentUser = { name: "users/test" };
    view.rerender(
      <GlobalNoteEditorProvider>
        <Trigger />
      </GlobalNoteEditorProvider>,
    );

    await act(async () => {
      resolveEditorLoad({ default: MockNoteEditor });
    });

    expect(screen.queryByRole("dialog", DIALOG)).not.toBeInTheDocument();
  });

  it("does not resurrect an open composer after the same user signs out and back in", async () => {
    const view = renderProvider();
    fireEvent.click(screen.getByRole("button", { name: "Open editor" }));
    await screen.findByRole("dialog", DIALOG);

    mocks.currentUser = undefined;
    view.rerender(
      <GlobalNoteEditorProvider>
        <Trigger />
      </GlobalNoteEditorProvider>,
    );
    await expectClosed();

    mocks.currentUser = { name: "users/test" };
    view.rerender(
      <GlobalNoteEditorProvider>
        <Trigger />
      </GlobalNoteEditorProvider>,
    );

    expect(screen.queryByRole("dialog", DIALOG)).not.toBeInTheDocument();
  });

  it("does not assign a global Compose shortcut", () => {
    renderProvider(<span />);

    fireEvent.keyDown(window, { key: "m", shiftKey: true, metaKey: true });

    expect(mocks.loadNoteEditor).not.toHaveBeenCalled();
  });

  it.each([
    "/?creator=steven",
    "/archived",
    "/inbox",
    "/setting",
    "/views",
    "/notes/abc",
    "/notes/shares/token",
    "/403",
    "/404",
    "/unknown",
  ])("creates a note from %s", async (pathname) => {
    mocks.pathname = pathname;
    await openViaTrigger();

    expect(mocks.editorProps).toMatchObject({ cacheKey: "global-note-editor" });
    expect(mocks.editorProps).not.toHaveProperty("defaultSpace");
  });
});

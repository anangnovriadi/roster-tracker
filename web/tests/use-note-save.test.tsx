import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNoteSave } from "@/components/NoteEditor/hooks/useNoteSave";

const mocks = vi.hoisted(() => ({
  dispatch: vi.fn(),
  markNewNote: vi.fn(),
  noteSave: vi.fn(),
  newNoteName: null as string | null,
  timestamps: {} as { createTime?: Date; updateTime?: Date },
}));

vi.mock("@/components/NoteEditor/services", () => ({
  errorService: { getErrorMessage: () => "save failed" },
  noteService: { save: mocks.noteSave },
  validationService: { canSave: () => ({ valid: true }) },
}));

vi.mock("@/components/NoteEditor/state", () => ({
  useEditorContext: () => ({
    actions: {
      reset: () => ({ type: "reset" }),
      setLoading: (key: string, value: boolean) => ({ type: "set-loading", key, value }),
      setJustSaved: (value: boolean) => ({ type: "set-just-saved", value }),
      setMetadata: () => ({ type: "set-metadata" }),
      setTimestamps: () => ({ type: "set-timestamps" }),
    },
    dispatch: mocks.dispatch,
    getState: () => ({ ui: { justSaved: false }, timestamps: mocks.timestamps }),
  }),
}));

vi.mock("@/contexts/NewNoteContext", () => ({
  useNewNote: () => ({ newNoteName: mocks.newNoteName, markNewNote: mocks.markNewNote }),
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

describe("useNoteSave", () => {
  beforeEach(() => {
    mocks.dispatch.mockReset();
    mocks.markNewNote.mockReset();
    mocks.noteSave.mockReset();
    mocks.newNoteName = null;
    mocks.timestamps = {};
    mocks.markNewNote.mockImplementation((value: string | null | ((current: string | null) => string | null)) => {
      mocks.newNoteName = typeof value === "function" ? value(mocks.newNoteName) : value;
    });
  });

  it("invalidates scoped attachment libraries after a note save", async () => {
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/new" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const discardDraft = vi.fn();
    const { result } = renderHook(() => useNoteSave({ discardDraft }), { wrapper });

    await act(async () => result.current());

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["attachments", "list"] });
    expect(discardDraft).toHaveBeenCalledOnce();
    expect(mocks.markNewNote).toHaveBeenCalledWith("notes/new");
  });

  it("refreshes the parent note total after creating a comment", async () => {
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/comment" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ parentNoteName: "notes/parent", discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["notes", "comments", "notes/parent"] });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["notes", "detail", "notes/parent"] });
  });

  it("holds a saved confirmation before a closing host resets", async () => {
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/existing" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const onConfirm = vi.fn();
    const { result } = renderHook(() => useNoteSave({ noteName: "notes/existing", discardDraft: vi.fn(), onConfirm }), { wrapper });

    await act(async () => result.current());

    const types = mocks.dispatch.mock.calls.map(([action]) => action);
    const savedOn = types.findIndex((a) => a.type === "set-just-saved" && a.value === true);
    const reset = types.findIndex((a) => a.type === "reset");
    expect(savedOn).toBeGreaterThan(-1);
    expect(reset).toBeGreaterThan(savedOn);
    expect(onConfirm).toHaveBeenCalledWith("notes/existing");
  });

  it("clears the new note hoist after editing that note", async () => {
    mocks.newNoteName = "notes/new";
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/new" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ noteName: "notes/new", discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    expect(mocks.newNoteName).toBeNull();
  });

  it("preserves the new note hoist when editing another note", async () => {
    mocks.newNoteName = "notes/new";
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/existing" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ noteName: "notes/existing", discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    expect(mocks.newNoteName).toBe("notes/new");
  });

  it("preserves a newer marker created while the edit save is pending", async () => {
    mocks.newNoteName = "notes/new";
    let finishSave!: (value: { hasChanges: boolean; noteName: string }) => void;
    mocks.noteSave.mockReturnValue(
      new Promise((resolve) => {
        finishSave = resolve;
      }),
    );
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ noteName: "notes/new", discardDraft: vi.fn() }), { wrapper });

    await act(async () => {
      const save = result.current();
      mocks.newNoteName = "notes/newer";
      finishSave({ hasChanges: true, noteName: "notes/new" });
      await save;
    });

    expect(mocks.newNoteName).toBe("notes/newer");
  });

  it("preserves the new note hoist when the edit has no changes", async () => {
    mocks.newNoteName = "notes/new";
    mocks.noteSave.mockResolvedValue({ hasChanges: false, noteName: "notes/new" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ noteName: "notes/new", discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    expect(mocks.markNewNote).not.toHaveBeenCalled();
  });

  it("stamps an untouched default create time with the time of saving", async () => {
    vi.useFakeTimers({ now: new Date(2026, 9, 6, 13, 17, 40), toFake: ["Date"] });
    try {
      // Derived when the day was selected; the same reference stays in state across saves.
      const defaultCreateTime = new Date(2026, 9, 1, 13, 17, 23);
      mocks.timestamps = { createTime: defaultCreateTime, updateTime: defaultCreateTime };
      mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/new" });
      const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
      const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
      const { result } = renderHook(() => useNoteSave({ defaultCreateTime, discardDraft: vi.fn() }), { wrapper });

      await act(async () => result.current());
      vi.setSystemTime(new Date(2026, 9, 6, 13, 17, 55));
      await act(async () => result.current());

      const sent = mocks.noteSave.mock.calls.map(([state]) => state.timestamps.createTime as Date);
      expect(sent[0]).toEqual(new Date(2026, 9, 1, 13, 17, 40));
      expect(sent[1]).toEqual(new Date(2026, 9, 1, 13, 17, 55));
      expect(defaultCreateTime).toEqual(new Date(2026, 9, 1, 13, 17, 23));
    } finally {
      vi.useRealTimers();
    }
  });

  it("sends a manually edited create time unchanged", async () => {
    const defaultCreateTime = new Date(2026, 9, 1, 13, 17, 23);
    const edited = new Date(2026, 8, 20, 9, 30, 0);
    mocks.timestamps = { createTime: edited, updateTime: edited };
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/new" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ defaultCreateTime, discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    expect(mocks.noteSave.mock.calls[0][0].timestamps.createTime).toBe(edited);
  });

  it("does not hold the in-place composer after saving a new note", async () => {
    mocks.noteSave.mockResolvedValue({ hasChanges: true, noteName: "notes/new" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useNoteSave({ discardDraft: vi.fn() }), { wrapper });

    await act(async () => result.current());

    const savedOn = mocks.dispatch.mock.calls.some(([action]) => action.type === "set-just-saved" && action.value === true);
    expect(savedOn).toBe(false);
  });
});

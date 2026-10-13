import { createContext, type ReactNode, useContext, useState } from "react";
import { useLocation } from "react-router-dom";
import { ROUTES, resolveCollectionRoute } from "@/router/routes";

export type NoteTimeBasis = "create_time" | "update_time";

/** Upper bound on feed columns, in display order. 1 = single reading column; 0 = as many as fit. */
export const MAX_COLUMNS_VALUES = [1, 2, 3, 0] as const;
export type NoteMaxColumns = (typeof MAX_COLUMNS_VALUES)[number];

interface ViewState {
  orderByTimeAsc: boolean;
  timeBasis?: NoteTimeBasis;
  sortTimeField?: NoteTimeBasis;
  compactMode: boolean;
  linkPreview: boolean;
  maxColumns: NoteMaxColumns;
}

interface ViewContextValue {
  orderByTimeAsc: boolean;
  timeBasis: NoteTimeBasis;
  compactMode: boolean;
  linkPreview: boolean;
  maxColumns: NoteMaxColumns;
  setOrderByTimeAsc: (value: boolean) => void;
  setTimeBasis: (field: NoteTimeBasis) => void;
  setCompactMode: (value: boolean) => void;
  setLinkPreview: (value: boolean) => void;
  setMaxColumns: (value: NoteMaxColumns) => void;
}

const ViewContext = createContext<ViewContextValue | null>(null);

const LOCAL_STORAGE_KEY = "notes-view-setting";

const DEFAULT_VIEW_STATE: ViewState = { orderByTimeAsc: false, compactMode: false, linkPreview: true, maxColumns: 1 };

export function ViewProvider({ children, enabled = true }: { children: ReactNode; enabled?: boolean }) {
  const getInitialState = (): ViewState => {
    try {
      const cached = localStorage.getItem(LOCAL_STORAGE_KEY);
      if (cached) {
        const data = JSON.parse(cached) as Partial<ViewState>;
        const cachedTimeBasis = data.timeBasis ?? data.sortTimeField;
        const timeBasis = cachedTimeBasis === "create_time" || cachedTimeBasis === "update_time" ? cachedTimeBasis : undefined;
        const maxColumns = MAX_COLUMNS_VALUES.includes(data.maxColumns as NoteMaxColumns)
          ? (data.maxColumns as NoteMaxColumns)
          : DEFAULT_VIEW_STATE.maxColumns;
        return {
          orderByTimeAsc: Boolean(data.orderByTimeAsc ?? DEFAULT_VIEW_STATE.orderByTimeAsc),
          timeBasis,
          compactMode: Boolean(data.compactMode ?? DEFAULT_VIEW_STATE.compactMode),
          linkPreview: Boolean(data.linkPreview ?? DEFAULT_VIEW_STATE.linkPreview),
          maxColumns,
        };
      }
    } catch (error) {
      console.warn("Failed to load view settings from localStorage:", error);
    }
    return { ...DEFAULT_VIEW_STATE };
  };

  const [viewState, setViewState] = useState(getInitialState);
  // Keep the saved Timeline preferences while other views use their default presentation.
  const activeState = enabled ? viewState : DEFAULT_VIEW_STATE;
  const timeBasis = activeState.timeBasis ?? "create_time";

  const persistToStorage = (newState: ViewState) => {
    try {
      localStorage.setItem(LOCAL_STORAGE_KEY, JSON.stringify(newState));
    } catch (error) {
      console.warn("Failed to persist view settings:", error);
    }
  };

  const updateState = (patch: Partial<ViewState>) => {
    setViewState((prev) => {
      const newState = { ...prev, ...patch };
      persistToStorage(newState);
      return newState;
    });
  };

  const setOrderByTimeAsc = (value: boolean) => updateState({ orderByTimeAsc: value });
  const setTimeBasis = (field: NoteTimeBasis) => updateState({ timeBasis: field });
  const setCompactMode = (value: boolean) => updateState({ compactMode: value });
  const setLinkPreview = (value: boolean) => updateState({ linkPreview: value });
  const setMaxColumns = (value: NoteMaxColumns) => updateState({ maxColumns: value });

  return (
    <ViewContext.Provider
      value={{
        orderByTimeAsc: activeState.orderByTimeAsc,
        timeBasis,
        compactMode: activeState.compactMode,
        linkPreview: activeState.linkPreview,
        maxColumns: activeState.maxColumns,
        setOrderByTimeAsc,
        setTimeBasis,
        setCompactMode,
        setLinkPreview,
        setMaxColumns,
      }}
    >
      {children}
    </ViewContext.Provider>
  );
}

/** Timeline preferences also apply to its sidebar statistics and date filters. */
export function TimelineViewProvider({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const route = resolveCollectionRoute(pathname);
  const collectionPath = route.pathname.toLowerCase();
  const enabled = route.isCollection && (collectionPath === ROUTES.HOME || collectionPath === ROUTES.EXPLORE);
  return <ViewProvider enabled={enabled}>{children}</ViewProvider>;
}

export function useView() {
  const context = useContext(ViewContext);
  if (!context) {
    throw new Error("useView must be used within ViewProvider");
  }
  return context;
}

// Read the link-preview preference from deep inside the markdown renderer, which
// can render outside a ViewProvider (tests, isolated previews). Defaults to
// enabled when no provider is present, preserving the historical behavior.
export function useLinkPreviewEnabled() {
  return useContext(ViewContext)?.linkPreview ?? DEFAULT_VIEW_STATE.linkPreview;
}

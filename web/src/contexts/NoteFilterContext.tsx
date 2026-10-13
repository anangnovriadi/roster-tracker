import { uniqBy } from "lodash-es";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";

export type FilterFactor =
  | "tagSearch"
  | "visibility"
  | "contentSearch"
  | "celSearch"
  | "displayTime"
  | "pinned"
  | "property.hasLink"
  | "property.hasTaskList"
  | "property.hasCode"
  | "property.hasLocation";

export interface NoteFilter {
  factor: FilterFactor;
  value: string;
}

export const getNoteFilterKey = (filter: NoteFilter): string => `${filter.factor}:${filter.value}`;

export const parseFilterQuery = (query: string | null): NoteFilter[] => {
  if (!query) return [];
  try {
    return query.split(",").map((filterStr) => {
      const separatorIndex = filterStr.indexOf(":");
      const factor = separatorIndex === -1 ? filterStr : filterStr.slice(0, separatorIndex);
      const value = separatorIndex === -1 ? "" : filterStr.slice(separatorIndex + 1);
      return {
        factor: factor as FilterFactor,
        value: decodeURIComponent(value || ""),
      };
    });
  } catch {
    return [];
  }
};

export const stringifyFilters = (filters: NoteFilter[]): string => {
  return filters.map((filter) => `${filter.factor}:${encodeURIComponent(filter.value)}`).join(",");
};

/** The `?filter=` search string that carries these filters in a URL, or "" when there are none. */
export const getFilterSearch = (filters: NoteFilter[]): string => {
  const filterQuery = stringifyFilters(filters);
  return filterQuery ? `?${new URLSearchParams({ filter: filterQuery })}` : "";
};

/** Search filters carry the user's query itself (plain words or a CEL expression), as opposed to facets. */
export const isSearchFilter = (filter: NoteFilter): boolean => filter.factor === "contentSearch" || filter.factor === "celSearch";

export const replaceFiltersByFactor = (filters: NoteFilter[], factor: FilterFactor, replacements: NoteFilter[]): NoteFilter[] => [
  ...filters.filter((filter) => filter.factor !== factor),
  ...replacements,
];

interface NoteFilterContextValue {
  filters: NoteFilter[];
  noteView: string | undefined;
  hasActiveFilters: boolean;
  getFiltersByFactor: (factor: FilterFactor) => NoteFilter[];
  setFilters: (filters: NoteFilter[]) => void;
  addFilter: (filter: NoteFilter) => void;
  removeFilter: (predicate: (f: NoteFilter) => boolean) => void;
  removeFiltersByFactor: (factor: FilterFactor) => void;
  clearAllFilters: () => void;
  setNoteView: (noteView?: string) => void;
  hasFilter: (filter: NoteFilter) => boolean;
}

const NoteFilterContext = createContext<NoteFilterContextValue | null>(null);

export function NoteFilterProvider({ children }: { children: ReactNode }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const locationStateRef = useRef(useLocation().state);
  const skipStoreSyncRef = useRef(false);
  const lastSyncedUrlRef = useRef("");
  const lastSyncedStoreRef = useRef("");

  // Initialize from URL
  const [filters, setFiltersState] = useState<NoteFilter[]>(() => {
    return parseFilterQuery(searchParams.get("filter"));
  });
  const [noteView, setNoteViewState] = useState<string | undefined>(undefined);

  // Sync URL to state when URL changes externally
  useEffect(() => {
    const filterParam = searchParams.get("filter") || "";
    if (filterParam !== lastSyncedUrlRef.current) {
      // The store still contains the previous page’s filters in this commit.
      // Do not write them back over the destination URL before state catches up.
      skipStoreSyncRef.current = true;
      lastSyncedUrlRef.current = filterParam;
      const newFilters = parseFilterQuery(filterParam);
      setFiltersState(newFilters);
      lastSyncedStoreRef.current = stringifyFilters(newFilters);
    }
  }, [searchParams]);

  // Sync state to URL when state changes
  useEffect(() => {
    if (skipStoreSyncRef.current) {
      skipStoreSyncRef.current = false;
      return;
    }
    const storeString = stringifyFilters(filters);
    if (storeString !== lastSyncedStoreRef.current && storeString !== lastSyncedUrlRef.current) {
      lastSyncedStoreRef.current = storeString;
      const newParams = new URLSearchParams(searchParams);
      if (filters.length > 0) {
        newParams.set("filter", storeString);
      } else {
        newParams.delete("filter");
      }
      setSearchParams(newParams, { replace: true, state: locationStateRef.current });
      lastSyncedUrlRef.current = filters.length > 0 ? storeString : "";
    }
  }, [filters, searchParams, setSearchParams]);

  const getFiltersByFactor = useCallback((factor: FilterFactor) => filters.filter((f) => f.factor === factor), [filters]);

  const setFilters = useCallback((newFilters: NoteFilter[]) => {
    setFiltersState(newFilters);
  }, []);

  const addFilter = useCallback((filter: NoteFilter) => {
    setFiltersState((prev) => uniqBy([...prev, filter], getNoteFilterKey));
  }, []);

  const removeFilter = useCallback((predicate: (f: NoteFilter) => boolean) => {
    setFiltersState((prev) => prev.filter((f) => !predicate(f)));
  }, []);

  const removeFiltersByFactor = useCallback((factor: FilterFactor) => {
    setFiltersState((prev) => prev.filter((f) => f.factor !== factor));
  }, []);

  const clearAllFilters = useCallback(() => {
    setFiltersState([]);
    setNoteViewState(undefined);
  }, []);

  const setNoteView = useCallback((newNoteView?: string) => {
    setNoteViewState(newNoteView);
  }, []);

  const hasFilter = useCallback((filter: NoteFilter) => filters.some((f) => getNoteFilterKey(f) === getNoteFilterKey(filter)), [filters]);

  const hasActiveFilters = filters.length > 0 || noteView !== undefined;
  const value = useMemo(
    () => ({
      filters,
      noteView,
      hasActiveFilters,
      getFiltersByFactor,
      setFilters,
      addFilter,
      removeFilter,
      removeFiltersByFactor,
      clearAllFilters,
      setNoteView,
      hasFilter,
    }),
    [
      filters,
      noteView,
      hasActiveFilters,
      getFiltersByFactor,
      setFilters,
      addFilter,
      removeFilter,
      removeFiltersByFactor,
      clearAllFilters,
      setNoteView,
      hasFilter,
    ],
  );

  return <NoteFilterContext.Provider value={value}>{children}</NoteFilterContext.Provider>;
}

export function useNoteFilterContext() {
  const context = useContext(NoteFilterContext);
  if (!context) {
    throw new Error("useNoteFilterContext must be used within NoteFilterProvider");
  }
  return context;
}

// Alias for backwards compatibility during migration
export const useNoteFilter = useNoteFilterContext;

const NO_TERMS: string[] = [];

/**
 * The words of the active plain-text search, in the order the user typed them. Safe outside a
 * provider (share previews, tests), where there is no search and therefore nothing to match.
 */
export function useContentSearchTerms(): string[] {
  const filters = useContext(NoteFilterContext)?.filters;
  return useMemo(() => {
    const terms = (filters ?? []).filter((filter) => filter.factor === "contentSearch").map((filter) => filter.value);
    return terms.length > 0 ? terms : NO_TERMS;
  }, [filters]);
}

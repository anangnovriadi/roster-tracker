import { buildNotePath, ROUTES } from "@/router/routes";

export interface NoteNavigationState {
  from: string;
}

interface ResolveNoteDetailOriginOptions {
  noteArchived?: boolean;
}

interface ResolveNoteParentPageOptions {
  explicitParentPage?: string;
  pathname: string;
  search: string;
  noteName: string;
}

const normalizePathname = (pathname: string): string => {
  const normalized = pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
  return normalized.toLowerCase();
};

/** Whether the current route is any canonical or shared Note detail page. */
export const isNoteResourcePath = (pathname: string): boolean => {
  const normalizedPath = normalizePathname(pathname);
  const sharedNotePrefix = `${ROUTES.SHARED_NOTE}/`;
  if (normalizedPath.startsWith(sharedNotePrefix)) {
    const shareToken = normalizedPath.slice(sharedNotePrefix.length);
    return shareToken.length > 0 && !shareToken.includes("/");
  }

  const directNotePrefix = `${ROUTES.NOTE}/`;
  if (!normalizedPath.startsWith(directNotePrefix)) return false;
  const noteID = normalizedPath.slice(directNotePrefix.length);
  return noteID.length > 0 && !noteID.includes("/");
};

export const createNoteNavigationState = (from: string): NoteNavigationState => ({ from });

/** Reads the origin page out of router state; without one, a detail returns to its collection. */
export const resolveNoteDetailOrigin = (state: unknown, options: ResolveNoteDetailOriginOptions = {}): string => {
  const value = state && typeof state === "object" ? (state as { from?: unknown }) : undefined;
  const explicitParentPage = typeof value?.from === "string" ? value.from : undefined;
  return explicitParentPage || (options.noteArchived ? ROUTES.ARCHIVED : ROUTES.HOME);
};

/** Whether the current route is the canonical detail page for this note. */
export const isNoteDetailPath = (pathname: string, noteName: string): boolean => {
  const normalizedPath = normalizePathname(pathname);
  if (normalizedPath === buildNotePath(noteName).toLowerCase()) return true;
  return normalizedPath.startsWith(`${ROUTES.SHARED_NOTE}/`) && isNoteResourcePath(normalizedPath);
};

/**
 * Captures the list page that opened a note so canonical detail routes can
 * return to the real collection lens instead of assuming Home.
 */
export const resolveNoteParentPage = ({ explicitParentPage, pathname, search, noteName }: ResolveNoteParentPageOptions): string => {
  if (explicitParentPage !== undefined) return explicitParentPage || ROUTES.HOME;
  if (isNoteDetailPath(pathname, noteName)) return ROUTES.HOME;
  return `${pathname}${search}`;
};

/** Replaces only the note filter while preserving the rest of the origin query. */
export const withNoteFilter = (page: string, filter: string): string => {
  const [pathAndSearch] = page.split("#", 1);
  const questionMark = pathAndSearch.indexOf("?");
  const pathname = questionMark === -1 ? pathAndSearch : pathAndSearch.slice(0, questionMark);
  const search = questionMark === -1 ? "" : pathAndSearch.slice(questionMark + 1);
  const searchParams = new URLSearchParams(search);
  searchParams.set("filter", filter);
  return `${pathname || ROUTES.HOME}?${searchParams.toString()}`;
};

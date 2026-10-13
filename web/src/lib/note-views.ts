import { ROUTES, resolveCollectionRoute } from "@/router/routes";

export type NoteScope = "home" | "explore" | "archived";

export const getNoteViewId = (name: string): string => {
  const parts = name.split("/");
  return parts.length === 4 ? parts[3] : name;
};

const cleanPathname = (value: string): string => {
  const pathname = value.split(/[?#]/, 1)[0] || ROUTES.HOME;
  return pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
};

/** Lower-cased global pathname, so a Space-scoped URL compares like its global twin. */
const comparablePathname = (pathname: string): string => resolveCollectionRoute(pathname).pathname.toLowerCase();

export const isNoteScopeRoute = (pathname: string): boolean => {
  const comparablePath = comparablePathname(pathname);
  return comparablePath === ROUTES.HOME || comparablePath === ROUTES.EXPLORE || comparablePath === ROUTES.ARCHIVED;
};

/** `/calendar` and any month or day beneath it. */
export const isCalendarRoute = (pathname: string): boolean => {
  const comparablePath = comparablePathname(pathname);
  return comparablePath === ROUTES.CALENDAR || comparablePath.startsWith(`${ROUTES.CALENDAR}/`);
};

/**
 * Routes that render a note collection the sidebar can narrow. Views, calendar days,
 * and tags apply in place on all of them.
 */
export const isNoteCollectionRoute = (pathname: string): boolean => isNoteScopeRoute(pathname) || isCalendarRoute(pathname);

interface ResolveNoteScopeOptions {
  detailFrom?: string;
  noteArchived?: boolean;
  fallback?: NoteScope;
}

export const resolveNoteScope = (pathname: string, options: ResolveNoteScopeOptions = {}): NoteScope => {
  const cleanPath = cleanPathname(pathname);
  const comparablePath = comparablePathname(cleanPath);
  if (comparablePath === ROUTES.EXPLORE) return "explore";
  if (comparablePath === ROUTES.ARCHIVED) return "archived";
  if (comparablePath === ROUTES.HOME) return "home";

  if (comparablePath.startsWith(`${ROUTES.NOTE}/`) && options.detailFrom) {
    return resolveNoteScope(options.detailFrom, {
      fallback: options.fallback,
    });
  }

  if (comparablePath.startsWith(`${ROUTES.NOTE}/`) && options.noteArchived) {
    return "archived";
  }

  return options.fallback ?? "home";
};

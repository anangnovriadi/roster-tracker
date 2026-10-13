import { matchPath } from "react-router-dom";
import { isCalendarRoute, isMemoScopeRoute, type MemoScope, resolveMemoScope } from "@/lib/memo-views";
import { getCollectionCreator, ROUTES, resolveCollectionRoute, withCollectionCreator } from "@/router/routes";

export type SidebarRouteKind = MemoScope | "calendar" | "attachments" | "settings" | "memo" | "common";

export type RouteSearchScope = "route-collection" | "user-collection" | "all";

export interface RouteActionPolicy {
  searchScope: RouteSearchScope;
  /** When absent, Quick Find stays on the current route. */
  searchDestination?: string;
}

export const getSidebarRouteKind = (path: string): SidebarRouteKind => {
  const normalizedPath = resolveCollectionRoute(path).pathname;
  if (isMemoScopeRoute(normalizedPath)) return resolveMemoScope(normalizedPath);
  if (isCalendarRoute(normalizedPath)) return "calendar";
  if (matchPath(ROUTES.ATTACHMENTS, normalizedPath)) return "attachments";
  if (matchPath(ROUTES.SETTING, normalizedPath)) return "settings";
  if (matchPath(`${ROUTES.NOTE}/:uid`, normalizedPath) || matchPath(`${ROUTES.SHARED_MEMO}/:token`, normalizedPath)) return "memo";
  return "common";
};

/**
 * Keeps read/search scope explicit at route boundaries. Compose placement needs no
 * policy: the URL only carries a Space on collection pages.
 */
export const getRouteActionPolicy = (path: string, search = ""): RouteActionPolicy => {
  const kind = getSidebarRouteKind(path);

  // The calendar is where notes are read, so it filters in place.
  if (kind === "calendar") return { searchScope: "route-collection" };

  // Attachments is not a note list, so a search leaves for the same collection's calendar.
  if (kind === "attachments") {
    const creator = getCollectionCreator(search);
    return { searchScope: "route-collection", searchDestination: `${ROUTES.CALENDAR}${withCollectionCreator("", creator)}` };
  }

  return { searchScope: "all", searchDestination: ROUTES.CALENDAR };
};

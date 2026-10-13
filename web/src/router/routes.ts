import { matchPath } from "react-router-dom";

export const ROUTES = {
  HOME: "/",
  ATTACHMENTS: "/attachments",
  ARCHIVED: "/archived",
  CALENDAR: "/calendar",
  SETTING: "/setting",
  EXPLORE: "/explore",
  USER_PROFILE: "/u/:username",
  AUTH: "/auth",
  AUTH_ADMIN: "/auth/admin",
  AUTH_CALLBACK: "/auth/callback",
  NOTE: "/notes",
  SHARED_MEMO: "/notes/shares",
} as const;

/**
 * The page of a note. Notes are named `memos/{uid}` by the API but live under `/notes/` in
 * the app's URLs; `/memos/...` links from before the rename redirect here.
 */
export const buildNotePath = (memoName: string): string => `${ROUTES.NOTE}/${memoName.replace(/^memos\//, "")}`;

/** Router pattern for the calendar: month and day are optional so `/calendar` can redirect. */
export const CALENDAR_ROUTE_PATTERN = `${ROUTES.CALENDAR}/:year?/:month?/:day?`;

export type RouteKey = keyof typeof ROUTES;
export type RoutePath = (typeof ROUTES)[RouteKey];

/** Pages that list a memo collection and carry its creator filter. */
const COLLECTION_ROUTE_PATTERNS = [ROUTES.HOME, ROUTES.EXPLORE, ROUTES.ATTACHMENTS, CALENDAR_ROUTE_PATTERN];

const isCollectionPathname = (pathname: string): boolean =>
  COLLECTION_ROUTE_PATTERNS.some((path) => matchPath({ path, caseSensitive: false }, pathname) !== null);

export interface CollectionRoute {
  /** The normalized pathname. */
  pathname: string;
  isCollection: boolean;
}

export const resolveCollectionRoute = (path: string): CollectionRoute => {
  const pathname = (path.split(/[?#]/, 1)[0] || ROUTES.HOME).replace(/\/+$/, "") || ROUTES.HOME;
  return { pathname, isCollection: isCollectionPathname(pathname) };
};

/** An explicit creator is a URL-owned collection filter. */
export const getCollectionCreator = (search: string): string | undefined => {
  const username = new URLSearchParams(search).get("creator");
  return username || undefined;
};

export const withCollectionCreator = (search: string, username?: string): string => {
  const params = new URLSearchParams(search);
  if (username) params.set("creator", username);
  else params.delete("creator");
  const query = params.toString();
  return query ? `?${query}` : "";
};

/** The canonical collection for one creator. */
export const getCreatorHomePath = (username: string): string => `${ROUTES.HOME}${withCollectionCreator("", username)}`;

/** Home is personal; Explore is all readable creators. Other collection views keep explicit creator filters. */
export const getCollectionHomePath = (location: { pathname: string; search: string }): string => {
  const route = resolveCollectionRoute(location.pathname);
  return route.pathname === ROUTES.EXPLORE ||
    (route.isCollection && route.pathname !== ROUTES.HOME && !getCollectionCreator(location.search))
    ? ROUTES.EXPLORE
    : ROUTES.HOME;
};

/** Carries creator scope and collection filters between views, excluding view-specific query state. */
export const collectionNavigationPath = (
  pathname: string,
  location: { pathname: string; search: string },
  currentUsername?: string,
): string => {
  const route = resolveCollectionRoute(location.pathname);
  const creator =
    route.pathname === ROUTES.EXPLORE
      ? undefined
      : ((route.isCollection ? getCollectionCreator(location.search) : undefined) ??
        (!route.isCollection || route.pathname === ROUTES.HOME ? currentUsername : undefined));
  const filter = route.isCollection ? new URLSearchParams(location.search).get("filter") : null;
  const search = filter ? new URLSearchParams({ filter }).toString() : "";
  // A memo list without a creator is Explore; this also sends guests there from any page.
  if (pathname === ROUTES.HOME) {
    const home = creator ? ROUTES.HOME : ROUTES.EXPLORE;
    return `${home}${withCollectionCreator(search, creator === currentUsername ? undefined : creator)}`;
  }
  return `${pathname}${withCollectionCreator(search, creator)}`;
};

/** A creator change keeps the current collection view. */
export const getCreatorSwitchPath = (
  location: { pathname: string; search: string; hash?: string },
  username?: string,
  currentUsername?: string,
): string => {
  const route = resolveCollectionRoute(location.pathname);
  const isMemoList = route.pathname === ROUTES.HOME || route.pathname === ROUTES.EXPLORE || !route.isCollection;
  const pathname = isMemoList ? (username ? ROUTES.HOME : ROUTES.EXPLORE) : location.pathname;
  const creator = username === currentUsername && isMemoList ? undefined : username;
  return `${pathname}${withCollectionCreator(location.search, creator)}${location.hash || ""}`;
};

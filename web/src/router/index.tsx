import { createBrowserRouter, Navigate, type RouteObject, useLocation } from "react-router-dom";
import App from "@/App";
import { ChunkLoadErrorFallback } from "@/components/ErrorBoundary";
import MainLayout from "@/layouts/MainLayout";
import RootLayout from "@/layouts/RootLayout";
import { lazyWithReload } from "@/utils/lazy";
import { RequireAuthRoute, RequireFullInitializationRoute, RequireGuestRoute, RequireInstanceInitializationRoute } from "./guards";
import { LegacyMemoRedirect } from "./LegacyMemoRedirect";
import { LegacyProfileRedirect } from "./LegacyProfileRedirect";
import { CALENDAR_ROUTE_PATTERN, ROUTES } from "./routes";

const AdminSignIn = lazyWithReload(() => import("@/pages/AdminSignIn"));
const AuthCallback = lazyWithReload(() => import("@/pages/AuthCallback"));
const Calendar = lazyWithReload(() => import("@/pages/Calendar"));
const MemoDetail = lazyWithReload(() => import("@/pages/MemoDetail"));
const NotFound = lazyWithReload(() => import("@/pages/NotFound"));
const PermissionDenied = lazyWithReload(() => import("@/pages/PermissionDenied"));
const Attachments = lazyWithReload(() => import("@/pages/Attachments"));
const Setting = lazyWithReload(() => import("@/pages/Setting"));
const SignIn = lazyWithReload(() => import("@/pages/SignIn"));

/** The calendar is the home of Roster Tracker; the old feed URLs land on it with their filters. */
const CalendarRedirect = () => {
  const location = useLocation();
  return <Navigate to={{ pathname: ROUTES.CALENDAR, search: location.search, hash: location.hash }} replace />;
};

// Backward compatibility alias.
export const Routes = ROUTES;
export { ROUTES };

/**
 * Static route configuration. Exported so tests can assert on the tree shape
 * (e.g. that `/auth/callback` stays outside the guest-only guard subtree) and
 * so integration tests can drive a `createMemoryRouter` over the same tree.
 */
export const routeConfig: RouteObject[] = [
  {
    path: "/",
    element: <App />,
    errorElement: <ChunkLoadErrorFallback />,
    children: [
      {
        path: Routes.AUTH,
        children: [
          // The OAuth callback must run regardless of the current session — an
          // authenticated tab elsewhere must not block it from consuming its
          // one-time OAuth state. Keep it outside the guest-only subtree.
          { path: "callback", element: <AuthCallback /> },
          {
            element: <RequireInstanceInitializationRoute />,
            children: [
              {
                element: <RequireGuestRoute />,
                children: [
                  { path: "", element: <SignIn /> },
                  { path: "admin", element: <AdminSignIn /> },
                ],
              },
            ],
          },
        ],
      },
      // Backward compatibility: the old `/home` URL now lives at `/`.
      { path: "home", element: <Navigate to={Routes.HOME} replace /> },
      {
        element: <RootLayout />,
        children: [
          {
            element: <MainLayout />,
            children: [
              { index: true, element: <CalendarRedirect /> },
              { path: Routes.EXPLORE, element: <CalendarRedirect /> },
              // Like Timeline, the calendar is public: guests see the memos readable to them.
              { path: CALENDAR_ROUTE_PATTERN, element: <Calendar /> },
              { path: Routes.USER_PROFILE, element: <LegacyProfileRedirect /> },
            ],
          },
          { path: "notes/:uid", element: <MemoDetail /> },
          { path: "notes/shares/:token", element: <MemoDetail /> },
          // Links from before notes moved from /memos/ to /notes/.
          { path: "memos/*", element: <LegacyMemoRedirect /> },
          {
            element: <RequireAuthRoute />,
            children: [
              {
                element: <RequireFullInitializationRoute />,
                children: [
                  { path: Routes.ATTACHMENTS, element: <Attachments /> },
                  { path: Routes.SETTING, element: <Setting /> },
                ],
              },
            ],
          },
          { path: "403", element: <PermissionDenied /> },
          { path: "404", element: <NotFound /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
];

const router = createBrowserRouter(routeConfig);

export default router;

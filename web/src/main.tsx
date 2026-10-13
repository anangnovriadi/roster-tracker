import "@github/relative-time-element";
import { QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import React, { useEffect, useRef } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "react-hot-toast";
import { RouterProvider } from "react-router-dom";
import "./i18n";
import "./index.css";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import PageLoader from "@/components/PageLoader";
import { TooltipProvider } from "@/components/ui/tooltip";
import { refreshAccessToken } from "@/connect";
import { AuthProvider, useAuth } from "@/contexts/AuthContext";
import { InstanceProvider, useInstance } from "@/contexts/InstanceContext";
import { useLiveNoteRefresh } from "@/hooks/useLiveNoteRefresh";
import { useTokenRefreshOnFocus } from "@/hooks/useTokenRefreshOnFocus";
import { queryClient } from "@/lib/query-client";
import router from "./router";
import { applyLocaleEarly } from "./utils/i18n";
import { applyThemeEarly } from "./utils/theme";

// Apply theme and locale early to prevent flash
applyThemeEarly();
applyLocaleEarly();

// Inner component that initializes contexts
function AppInitializer({ children }: { children: React.ReactNode }) {
  const { isIdentityInitialized, initialize: initAuth, currentUser } = useAuth();
  const { isProfileInitialized, initialize: initInstance } = useInstance();
  const initStartedRef = useRef(false);

  // Initialize on mount - run in parallel for better performance
  useEffect(() => {
    if (initStartedRef.current) return;
    initStartedRef.current = true;

    const init = async () => {
      await Promise.all([initInstance(), initAuth()]);
    };
    init();
  }, [initAuth, initInstance]);

  // Proactively refresh token on window focus to prevent 401 errors
  // Only enabled when user is authenticated
  useTokenRefreshOnFocus(refreshAccessToken, !!currentUser);

  // Live refresh: listen for note changes via SSE and invalidate caches.
  useLiveNoteRefresh();

  // Route loading and feed requests only need the verified identity and the
  // instance profile. Display-sensitive settings continue in the background;
  // PagedNoteList keeps note content hidden until privacy settings have settled.
  // Signing in re-checks the identity too; show a spinner rather than a blank page while it does.
  if (!isIdentityInitialized || !isProfileInitialized) {
    return <PageLoader className="min-h-dvh" />;
  }

  return <>{children}</>;
}

function Main() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <InstanceProvider>
          <AuthProvider>
            <TooltipProvider>
              <AppInitializer>
                <RouterProvider router={router} />
                <Toaster
                  position="top-right"
                  toastOptions={{
                    style: {
                      background: "var(--popover)",
                      color: "var(--popover-foreground)",
                    },
                  }}
                />
              </AppInitializer>
            </TooltipProvider>
          </AuthProvider>
        </InstanceProvider>
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

const container = document.getElementById("root");
const root = createRoot(container as HTMLElement);
root.render(<Main />);

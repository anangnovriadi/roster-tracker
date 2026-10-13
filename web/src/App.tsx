import { DirectionProvider } from "@base-ui/react/direction-provider";
import { Suspense, useEffect } from "react";
import { Outlet, ScrollRestoration } from "react-router-dom";
import PageLoader from "./components/PageLoader";
import { useInstance } from "./contexts/InstanceContext";
import { TimelineViewProvider } from "./contexts/ViewContext";
import { useUserLocale } from "./hooks/useUserLocale";
import { useUserTheme } from "./hooks/useUserTheme";
import { cleanupExpiredOAuthState } from "./utils/oauth";

const App = () => {
  const { generalSetting: instanceGeneralSetting } = useInstance();

  // Apply user preferences reactively
  const direction = useUserLocale();
  useUserTheme();

  // Clean up expired OAuth states on app initialization
  useEffect(() => {
    cleanupExpiredOAuthState();
  }, []);

  useEffect(() => {
    if (instanceGeneralSetting.additionalStyle) {
      const styleEl = document.createElement("style");
      styleEl.innerHTML = instanceGeneralSetting.additionalStyle;
      styleEl.setAttribute("type", "text/css");
      document.body.insertAdjacentElement("beforeend", styleEl);
    }
  }, [instanceGeneralSetting.additionalStyle]);

  useEffect(() => {
    if (instanceGeneralSetting.additionalScript) {
      const scriptEl = document.createElement("script");
      scriptEl.innerHTML = instanceGeneralSetting.additionalScript;
      document.head.appendChild(scriptEl);
    }
  }, [instanceGeneralSetting.additionalScript]);

  // Dynamic update metadata with customized profile
  useEffect(() => {
    if (!instanceGeneralSetting.customProfile) {
      return;
    }

    document.title = instanceGeneralSetting.customProfile.title;
    const link = document.querySelector("link[rel~='icon']") as HTMLLinkElement;
    const logoUrl = instanceGeneralSetting.customProfile.logoUrl;
    // The bundled logo is black; its favicon sits on a white badge so it shows on light and dark tabs.
    link.href = !logoUrl || logoUrl === "/logo.svg" ? "/favicon.svg" : logoUrl;
  }, [instanceGeneralSetting.customProfile]);

  return (
    <DirectionProvider direction={direction}>
      <TimelineViewProvider>
        <Suspense fallback={<PageLoader className="min-h-dvh" />}>
          <Outlet />
        </Suspense>
      </TimelineViewProvider>
      <ScrollRestoration />
    </DirectionProvider>
  );
};

export default App;

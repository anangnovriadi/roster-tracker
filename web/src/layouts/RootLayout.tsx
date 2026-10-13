import type { CSSProperties } from "react";
import { useEffect, useRef } from "react";
import { Outlet, useLocation, useSearchParams } from "react-router-dom";
import AppSidebar, {
  MobileAppHeader,
  MobileAppSidebar,
  QuickFindDialog,
  SIDEBAR_WIDTH_VAR,
  SidebarResizeHandle,
  useSidebarWidth,
} from "@/components/AppSidebar";
import SidebarToggleButton from "@/components/AppSidebar/SidebarToggleButton";
import { AppSidebarProvider, useAppSidebar } from "@/contexts/AppSidebarContext";
import { GlobalNoteEditorProvider } from "@/contexts/GlobalNoteEditorContext";
import { useInstance } from "@/contexts/InstanceContext";
import { NoteFilterProvider, useNoteFilterContext } from "@/contexts/NoteFilterContext";
import { SpaceProvider } from "@/contexts/SpaceContext";
import useMediaQuery from "@/hooks/useMediaQuery";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

const ROSTER_DEPLOY_URL = "https://github.com/anangnovriadi/roster-tracker#readme";

const DemoBanner = () => {
  const t = useTranslate();

  return (
    <div className="static w-full shrink-0 border-b border-border bg-muted/70 px-4 py-2 text-sm text-muted-foreground sm:px-6">
      <div className="mx-auto flex max-w-5xl flex-col items-start gap-1 sm:flex-row sm:items-center sm:justify-center sm:gap-2">
        <span className="font-medium text-foreground">{t("demo.banner-title")}</span>
        <span>{t("demo.banner-description")}</span>
        <a
          className="font-medium text-primary underline-offset-4 hover:underline"
          href={ROSTER_DEPLOY_URL}
          target="_blank"
          rel="noreferrer"
        >
          {t("demo.deploy-link")}
        </a>
      </div>
    </div>
  );
};

const RootLayoutContent = () => {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const md = useMediaQuery("md");
  const { profile } = useInstance();
  const { removeFilter } = useNoteFilterContext();
  const { desktopCollapsed } = useAppSidebar();
  const { pathname } = location;
  const prevPathnameRef = useRef<string | undefined>(undefined);
  const shellRef = useRef<HTMLDivElement>(null);
  const { width: sidebarWidth, minWidth, maxWidth, setWidth: setSidebarWidth } = useSidebarWidth();

  useEffect(() => {
    const prevPathname = prevPathnameRef.current;

    // When the route changes and there is no filter in the search params, remove all filters.
    if (prevPathname !== undefined && prevPathname !== pathname && !searchParams.has("filter")) {
      removeFilter(() => true);
    }

    prevPathnameRef.current = pathname;
  }, [pathname, searchParams, removeFilter]);

  // Roster Tracker is always public: signed-out visitors browse Explore and the roster,
  // so there is no private-instance gate here.

  return (
    <div ref={shellRef} className={"min-h-full w-full bg-background"} style={{ [SIDEBAR_WIDTH_VAR]: `${sidebarWidth}px` } as CSSProperties}>
      {md && !desktopCollapsed && (
        <div className="fixed inset-y-0 start-0 z-30 w-(--app-sidebar-width) border-e border-border/70">
          <AppSidebar collapsible />
          <SidebarResizeHandle
            width={sidebarWidth}
            minWidth={minWidth}
            maxWidth={maxWidth}
            onWidthChange={setSidebarWidth}
            targetRef={shellRef}
          />
        </div>
      )}
      {md && desktopCollapsed && <SidebarToggleButton className="fixed top-3 start-3 z-30 bg-background/90 backdrop-blur-sm" />}
      <MobileAppSidebar />
      <main
        className={cn(
          "flex min-h-full w-full min-w-0 flex-col items-center",
          desktopCollapsed ? "md:ps-14" : "md:ps-(--app-sidebar-width)",
        )}
      >
        <MobileAppHeader />
        {profile.demo && <DemoBanner />}
        <Outlet />
      </main>
      <QuickFindDialog />
    </div>
  );
};

const RootLayout = () => (
  <SpaceProvider>
    <NoteFilterProvider>
      <AppSidebarProvider>
        <GlobalNoteEditorProvider>
          <RootLayoutContent />
        </GlobalNoteEditorProvider>
      </AppSidebarProvider>
    </NoteFilterProvider>
  </SpaceProvider>
);

export default RootLayout;

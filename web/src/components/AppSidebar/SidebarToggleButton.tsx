import { PanelLeftIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

/** Collapses or expands the desktop sidebar; both states use the same icon. */
const SidebarToggleButton = ({ className }: { className?: string }) => {
  const t = useTranslate();
  const { desktopCollapsed, setDesktopCollapsed } = useAppSidebar();
  const label = t(desktopCollapsed ? "common.expand-sidebar" : "common.collapse-sidebar");

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-compact"
            className={cn("text-muted-foreground hover:text-foreground", className)}
            aria-label={label}
            aria-expanded={!desktopCollapsed}
            onClick={() => setDesktopCollapsed(!desktopCollapsed)}
          />
        }
      >
        <PanelLeftIcon className="size-4" strokeWidth={1.8} />
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  );
};

export default SidebarToggleButton;

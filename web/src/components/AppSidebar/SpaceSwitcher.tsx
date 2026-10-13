import { ChevronsUpDownIcon } from "lucide-react";
import { useRef, useState } from "react";
import AboutDialog from "@/components/AboutDialog";
import MemosLogo from "@/components/MemosLogo";
import UserMenu from "@/components/UserMenu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import { sidebarSurfaceVariants } from "./sidebar-layout";

function SpaceSwitcher({ className, size = "md" }: { className?: string; size?: "md" | "header" }) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const [aboutOpen, setAboutOpen] = useState(false);
  const [menuWidth, setMenuWidth] = useState<number>();
  const triggerRef = useRef<HTMLButtonElement>(null);
  // The header always names the app; the signed-in member is shown at the foot of the sidebar.
  const triggerLabel = t("common.memos");

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) return;
    const trigger = triggerRef.current;
    const sidebar = trigger?.closest("aside");
    if (!trigger || !sidebar) return;
    const sidebarRect = sidebar.getBoundingClientRect();
    const triggerRect = trigger.getBoundingClientRect();
    const inlineInset = Math.min(Math.abs(triggerRect.left - sidebarRect.left), Math.abs(sidebarRect.right - triggerRect.right));
    const width = Math.floor(sidebarRect.width - inlineInset * 2);
    if (width > 0) setMenuWidth(width);
  };

  return (
    <>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger
          ref={triggerRef}
          aria-label={triggerLabel}
          title={triggerLabel}
          className={cn(
            "text-start transition-colors hover:bg-sidebar-accent/65 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/40",
            sidebarSurfaceVariants({ role: size === "header" ? "headerBrand" : "mobileBrand" }),
            className,
          )}
        >
          <MemosLogo compact size={size === "header" ? "header" : "md"} />
          <ChevronsUpDownIcon
            aria-hidden="true"
            className={cn("shrink-0 text-muted-foreground/70", size === "header" ? "size-3" : "size-3.5")}
            strokeWidth={1.8}
          />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          side="bottom"
          sideOffset={4}
          className="w-[min(15rem,calc(100vw-1rem))] max-h-[min(27rem,calc(100dvh-5rem))] overflow-y-auto p-0"
          style={menuWidth ? { width: `${menuWidth}px` } : undefined}
        >
          <UserMenu onClose={() => setOpen(false)} onOpenAbout={() => setAboutOpen(true)} />
        </PopoverContent>
      </Popover>
      <AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
    </>
  );
}

export default SpaceSwitcher;

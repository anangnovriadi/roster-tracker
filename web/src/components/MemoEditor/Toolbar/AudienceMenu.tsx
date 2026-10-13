import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import VisibilityIcon from "@/components/VisibilityIcon";
import { useTranslate } from "@/utils/i18n";
import { getAssignableVisibilityOptions, getVisibilityOption } from "@/utils/memo";
import type { AudienceMenuProps } from "../types";

/** One quiet control for a memo's audience. */
const AudienceMenu = ({ value, onChange, disabled }: AudienceMenuProps) => {
  const t = useTranslate();
  const visibilityOptions = getAssignableVisibilityOptions({ hasSpacePlacement: false, current: value });
  const currentOption = getVisibilityOption(value);
  const visibilityLabel = currentOption ? t(currentOption.labelKey) : "";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="quiet" size="sm" className="min-w-0 max-w-64 shrink gap-1 text-muted-foreground" disabled={disabled} />}
        title={visibilityLabel}
        aria-label={visibilityLabel}
      >
        <span className="flex shrink-0 items-center gap-1">
          <VisibilityIcon visibility={value} className="size-3.5 text-current" />
          <span>{visibilityLabel}</span>
        </span>
        <ChevronDownIcon className="size-3 opacity-50" strokeWidth={1.8} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" size="sm" className="w-56" aria-label={t("common.visibility")}>
        {/* Two lines per audience: the description is the only place the app explains who can
            read a memo, and translations run long. The menu's fixed width makes long copy wrap
            instead of widening it; icon and check stay on the title's line. */}
        {visibilityOptions.map((option) => (
          <DropdownMenuItem key={option.value} onClick={() => onChange(option.value)}>
            <span aria-hidden className="flex h-4.5 w-5 shrink-0 items-center justify-center self-start">
              <VisibilityIcon visibility={option.value} className="size-3.5" />
            </span>
            <span className="flex min-w-0 flex-1 flex-col">
              <span>{t(option.labelKey)}</span>
              <span className="text-xs text-muted-foreground">{t(option.descriptionKey, { space: "" })}</span>
            </span>
            <span aria-hidden className="ms-auto flex h-4.5 w-3.5 shrink-0 items-center justify-center self-start">
              {option.value === value && <CheckIcon className="size-3.5" />}
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default AudienceMenu;

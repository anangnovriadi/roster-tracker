import { CheckIcon, ChevronDownIcon, GlobeIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { locales } from "@/i18n";
import { cn } from "@/lib/utils";
import { getLocaleDisplayName, useTranslate } from "@/utils/i18n";

interface LocaleListProps {
  value: Locale;
  onChange: (locale: Locale) => void;
  className?: string;
}

export const LocaleList = (props: LocaleListProps) => {
  const { value, onChange, className } = props;
  const t = useTranslate();

  return (
    <div
      className={cn("w-[var(--anchor-width)] min-w-48 max-w-[calc(100vw-2rem)] p-1", className)}
      role="listbox"
      aria-label={t("common.language")}
    >
      {locales.map((locale) => (
        <div
          key={locale}
          role="option"
          aria-selected={value === locale}
          className={cn(
            "flex h-8 w-full cursor-pointer items-center gap-2 rounded-sm px-2 text-left text-sm outline-hidden transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground",
            value === locale && "bg-accent/60",
          )}
          onClick={() => onChange(locale)}
        >
          <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
            <span className="truncate">{getLocaleDisplayName(locale)}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{locale}</span>
          </span>
          {value === locale ? <CheckIcon className="size-4 shrink-0 text-primary" /> : <span className="size-4 shrink-0" />}
        </div>
      ))}
    </div>
  );
};

interface LocalePickerProps {
  value: Locale;
  onChange: (locale: Locale) => void;
  className?: string;
}

const LocalePicker = (props: LocalePickerProps) => {
  const { value, onChange, className } = props;
  const [open, setOpen] = useState(false);

  const handleChange = (locale: Locale) => {
    onChange(locale);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<Button type="button" variant="outline" className={cn("w-full justify-between", className)} />}>
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <GlobeIcon className="size-4 shrink-0 text-muted-foreground" />
          <span className="truncate">{getLocaleDisplayName(value)}</span>
        </span>
        <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" />
      </PopoverTrigger>
      <PopoverContent align="start" className="p-0">
        <LocaleList value={value} onChange={handleChange} />
      </PopoverContent>
    </Popover>
  );
};

export default LocalePicker;

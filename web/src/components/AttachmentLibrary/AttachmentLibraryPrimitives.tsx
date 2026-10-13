import { ExternalLinkIcon } from "lucide-react";
import { Link, useLocation } from "react-router-dom";
import { createNoteNavigationState } from "@/components/NoteView/navigation";
import UserAvatar from "@/components/UserAvatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { extractUsernameFromName } from "@/lib/resource-names";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import type { User } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";

interface AttachmentMetadataLineProps {
  className?: string;
  items: Array<string | undefined>;
}

interface AttachmentSourceChipProps {
  noteName?: string;
}

interface AttachmentOpenButtonProps {
  className?: string;
  href: string;
}

interface AttachmentCreatorProps {
  creatorName?: string;
  user?: User;
}

export const AttachmentCreator = ({ creatorName, user }: AttachmentCreatorProps) => {
  if (!creatorName) return null;

  const username = user?.username || extractUsernameFromName(creatorName);
  const displayName = user?.displayName || username;

  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1.5 text-xs text-muted-foreground">
      <UserAvatar className="size-4" avatarUrl={user?.avatarUrl} name={displayName} />
      <span className="truncate">{displayName}</span>
    </span>
  );
};

export const AttachmentMetadataLine = ({ className, items }: AttachmentMetadataLineProps) => {
  const visibleItems = items.filter((item): item is string => Boolean(item));

  if (visibleItems.length === 0) {
    return null;
  }

  return (
    <div
      className={cn(
        "flex items-center gap-1.5 overflow-x-auto whitespace-nowrap text-xs text-muted-foreground [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        className,
      )}
    >
      {visibleItems.map((item, index) => (
        <span key={`${item}-${index}`} className="contents">
          {index > 0 && <span className="shrink-0 text-muted-foreground/50">•</span>}
          <span className="shrink-0">{item}</span>
        </span>
      ))}
    </div>
  );
};

export const AttachmentSourceChip = ({ noteName }: AttachmentSourceChipProps) => {
  const location = useLocation();
  const t = useTranslate();

  if (!noteName) {
    return (
      <Badge variant="warning" shape="pill" className="px-1.5 py-0.5 text-[11px]">
        {t("attachment-library.labels.not-linked")}
      </Badge>
    );
  }

  return (
    <Link
      to={buildNotePath(noteName)}
      state={createNoteNavigationState(`${location.pathname}${location.search}`)}
      className="inline-flex max-w-full items-center truncate rounded-full border border-border/60 bg-muted/30 px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted/50"
    >
      <span className="truncate">{t("attachment-library.labels.note")}</span>
    </Link>
  );
};

export const AttachmentOpenButton = ({ className, href }: AttachmentOpenButtonProps) => {
  const t = useTranslate();

  return (
    <Button
      render={<a href={href} target="_blank" rel="noreferrer" />}
      variant="ghost"
      size="icon"
      className={cn("size-7 shrink-0 rounded-full text-muted-foreground hover:text-foreground", className)}
    >
      <ExternalLinkIcon className="h-3.5 w-3.5" />
      <span className="sr-only">{t("attachment-library.actions.open")}</span>
    </Button>
  );
};

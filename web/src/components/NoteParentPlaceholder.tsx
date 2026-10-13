import { Button } from "@/components/ui/button";
import { useTranslate } from "@/utils/i18n";

export type NoteParentStatus = "loading" | "unavailable" | "error";

export default function NoteParentPlaceholder({ status, onRetry }: { status: NoteParentStatus; onRetry?: () => void }) {
  const t = useTranslate();
  return (
    <div className="px-2 py-1.5 text-xs text-muted-foreground" role="status">
      <p>{t(status === "loading" ? "note.parent-loading" : status === "error" ? "note.parent-load-error" : "note.parent-unavailable")}</p>
      {status === "unavailable" && <p className="mt-1 leading-5">{t("note.parent-unavailable-description")}</p>}
      {status === "error" && onRetry && (
        <Button variant="ghost" size="sm" className="mt-1" onClick={onRetry}>
          {t("search.retry")}
        </Button>
      )}
    </div>
  );
}

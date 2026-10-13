import { Button } from "@/components/ui/button";
import { useTranslate } from "@/utils/i18n";

interface AttachmentLibraryErrorStateProps {
  error?: Error;
  onRetry: () => void;
}

interface AttachmentLibrarySkeletonGridProps {
  count?: number;
}

export const AttachmentLibrarySkeletonGrid = ({ count = 8 }: AttachmentLibrarySkeletonGridProps) => {
  return (
    <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-3 xl:grid-cols-4">
      {Array.from({ length: count }).map((_, index) => (
        <div key={index} className="overflow-hidden rounded-[20px] border border-border/60 bg-background/90">
          <div className="aspect-[5/4] animate-pulse bg-muted/50" />
          <div className="space-y-2.5 p-3">
            <div className="h-4 w-2/3 animate-pulse rounded bg-muted/50" />
            <div className="h-3 w-1/2 animate-pulse rounded bg-muted/40" />
            <div className="h-7 w-full animate-pulse rounded bg-muted/40" />
          </div>
        </div>
      ))}
    </div>
  );
};

export const AttachmentLibraryErrorState = ({ error, onRetry }: AttachmentLibraryErrorStateProps) => {
  const t = useTranslate();

  return (
    <div className="rounded-[20px] border border-destructive/30 bg-destructive/5 p-6 text-center">
      <p className="text-sm text-muted-foreground">{error?.message ?? t("attachment-library.errors.load")}</p>
      <Button className="mt-4 rounded-full" onClick={onRetry}>
        {t("attachment-library.actions.retry")}
      </Button>
    </div>
  );
};

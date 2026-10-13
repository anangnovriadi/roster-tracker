import { LoaderCircleIcon } from "lucide-react";
import { useMemo, useState } from "react";
import {
  AttachmentDocumentRows,
  AttachmentLibraryEmptyState,
  AttachmentLibraryErrorState,
  AttachmentLibrarySkeletonGrid,
  AttachmentMediaGrid,
} from "@/components/AttachmentLibrary";
import PreviewImageDialog from "@/components/PreviewImageDialog";
import { Button } from "@/components/ui/button";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useSpaceContext } from "@/contexts/SpaceContext";
import { useAttachmentLibrary } from "@/hooks/useAttachmentLibrary";
import { useUsersByNames } from "@/hooks/useUserQueries";
import i18n from "@/i18n";
import { useTranslate } from "@/utils/i18n";

const Attachments = () => {
  const t = useTranslate();
  const { attachmentSection } = useAppSidebar();
  const { noteFilter } = useSpaceContext();
  const [previewState, setPreviewState] = useState({ open: false, initialIndex: 0 });
  const {
    documentItems,
    error,
    fetchNextPage,
    hasNextPage,
    isError,
    isFetching,
    isFetchingNextPage,
    isLoading,
    mediaGroups,
    mediaPreviewItems,
    refetch,
    stats,
  } = useAttachmentLibrary(i18n.language, noteFilter);
  const creatorNames = useMemo(
    () =>
      Array.from(
        new Set(
          [
            ...mediaGroups.flatMap((group) => group.items.map((item) => item.primaryAttachment.creator)),
            ...documentItems.map((item) => item.attachment.creator),
          ].filter(Boolean),
        ),
      ),
    [mediaGroups, documentItems],
  );
  const { data: creators } = useUsersByNames(creatorNames);

  const handlePreview = (itemId: string) => {
    const initialIndex = mediaPreviewItems.findIndex((item) => item.id === itemId);
    setPreviewState({ open: true, initialIndex: initialIndex >= 0 ? initialIndex : 0 });
  };

  const renderContent = () => {
    if (isLoading) {
      return <AttachmentLibrarySkeletonGrid />;
    }

    if (isError) {
      return <AttachmentLibraryErrorState error={error instanceof Error ? error : undefined} onRetry={() => refetch()} />;
    }

    if (attachmentSection === "all") {
      const total = stats.media + stats.documents;
      if (total === 0) return <div className="py-16 text-center text-sm text-muted-foreground">{t("attachment-library.empty.all")}</div>;
      return (
        <div className="space-y-9">
          {stats.media > 0 && (
            <section className="space-y-3">
              <h2 className="text-sm font-medium text-foreground">{t("attachment-library.tabs.media")}</h2>
              <AttachmentMediaGrid groups={mediaGroups} creators={creators} onPreview={handlePreview} />
            </section>
          )}
          {stats.documents > 0 && (
            <section className="space-y-3">
              <h2 className="text-sm font-medium text-foreground">{t("attachment-library.tabs.documents")}</h2>
              <AttachmentDocumentRows items={documentItems} creators={creators} />
            </section>
          )}
        </div>
      );
    }

    if (attachmentSection === "media") {
      if (stats.media === 0) return <AttachmentLibraryEmptyState tab="media" />;
      return <AttachmentMediaGrid groups={mediaGroups} creators={creators} onPreview={handlePreview} />;
    }

    if (attachmentSection === "documents") {
      if (stats.documents === 0) return <AttachmentLibraryEmptyState tab="documents" />;
      return <AttachmentDocumentRows items={documentItems} creators={creators} />;
    }

    return null;
  };

  return (
    <section className="@container min-h-full w-full pb-10 pt-3 md:pt-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 sm:gap-6 sm:px-6">
        <div className="min-h-[16rem] pt-1">
          {renderContent()}

          {hasNextPage && (
            <div className="mt-6 flex justify-center">
              <Button variant="outline" className="rounded-full px-4" onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
                {isFetchingNextPage ? <LoaderCircleIcon className="h-4 w-4 animate-spin" /> : null}
                {isFetchingNextPage ? t("resource.fetching-data") : t("note.load-more")}
              </Button>
            </div>
          )}

          {!isLoading && isFetching && !isFetchingNextPage && (
            <div className="mt-4 text-center text-xs text-muted-foreground">{t("resource.fetching-data")}</div>
          )}
        </div>
      </div>

      <PreviewImageDialog
        open={previewState.open}
        onOpenChange={(open) => setPreviewState((prev) => ({ ...prev, open }))}
        items={mediaPreviewItems}
        initialIndex={previewState.initialIndex}
      />
    </section>
  );
};

export default Attachments;

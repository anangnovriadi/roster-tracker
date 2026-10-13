import { timestampDate } from "@bufbuild/protobuf/wkt";
import dayjs from "dayjs";
import { useEffect, useMemo } from "react";
import { getAttachmentMetadata, isImageAttachment, isVideoAttachment } from "@/components/NoteMetadata/Attachment/attachmentHelpers";
import { useInfiniteAttachments } from "@/hooks/useAttachmentQueries";
import type { Attachment, ListAttachmentsRequest } from "@/types/proto/api/attachment_service_pb";
import { isMotionAttachment } from "@/utils/attachment";
import { useTranslate } from "@/utils/i18n";
import { type AttachmentVisualItem, buildAttachmentVisualItems, countLogicalAttachmentItems } from "@/utils/media-item";

export type AttachmentLibraryTab = "media" | "documents";

export interface AttachmentLibraryStats {
  media: number;
  documents: number;
}

export interface AttachmentLibraryListItem {
  attachment: Attachment;
  createdAt?: Date;
  createdLabel: string;
  fileTypeLabel: string;
  fileSizeLabel?: string;
  noteName?: string;
  sourceUrl: string;
}

export interface AttachmentLibraryMediaItem extends AttachmentVisualItem {
  primaryAttachment: Attachment;
  createdAt?: Date;
  createdLabel: string;
  fileTypeLabel: string;
  noteName?: string;
}

export interface AttachmentLibraryMonthGroup {
  key: string;
  label: string;
  items: AttachmentLibraryMediaItem[];
}

const PAGE_SIZE = 50;
const ATTACHMENT_LIBRARY_REQUEST = {
  pageSize: PAGE_SIZE,
  orderBy: "create_time desc",
};
const COMPLETE_ATTACHMENT_LIBRARY_REQUEST = {
  pageSize: 1000,
  orderBy: "create_time desc",
};

const useCompleteAttachments = (request: Partial<ListAttachmentsRequest>, enabled = true) => {
  const query = useInfiniteAttachments(request, { enabled });

  useEffect(() => {
    if (enabled && query.isSuccess && query.hasNextPage && !query.isFetchingNextPage) {
      void query.fetchNextPage();
    }
  }, [enabled, query.fetchNextPage, query.hasNextPage, query.isFetchingNextPage, query.isSuccess]);

  return {
    ...query,
    isComplete: enabled && query.isSuccess && !query.hasNextPage && !query.isFetchingNextPage,
  };
};

const sortByNewest = (a?: Date, b?: Date) => (b?.getTime() ?? 0) - (a?.getTime() ?? 0);

const isLinkedAttachment = (attachment: Attachment) => Boolean(attachment.note);

const isVisualAttachment = (attachment: Attachment) =>
  isImageAttachment(attachment) || isVideoAttachment(attachment) || isMotionAttachment(attachment);

export const buildAttachmentLibraryStats = (attachments: Attachment[]): AttachmentLibraryStats => {
  const linkedAttachments = attachments.filter(isLinkedAttachment);
  const visualAttachments = linkedAttachments.filter(isVisualAttachment);

  return {
    media: countLogicalAttachmentItems(visualAttachments),
    // Audio files have no section of their own, so they are listed with documents.
    documents: linkedAttachments.filter((attachment) => !isVisualAttachment(attachment)).length,
  };
};

const toCreatedAt = (attachment: Attachment): Date | undefined => {
  return attachment.createTime ? timestampDate(attachment.createTime) : undefined;
};

const formatCreatedAt = (date: Date | undefined, locale: string) => {
  if (!date) {
    return "—";
  }

  return date.toLocaleDateString(locale, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
};

const toLibraryListItem = (attachment: Attachment, locale: string): AttachmentLibraryListItem => {
  const createdAt = toCreatedAt(attachment);
  const { fileTypeLabel, fileSizeLabel } = getAttachmentMetadata(attachment);

  return {
    attachment,
    createdAt,
    createdLabel: formatCreatedAt(createdAt, locale),
    fileTypeLabel,
    fileSizeLabel,
    noteName: attachment.note,
    sourceUrl: attachment.externalLink || `${window.location.origin}/file/${attachment.name}/${attachment.filename}`,
  };
};

const toLibraryMediaItem = (item: AttachmentVisualItem, locale: string, livePhotoLabel: string): AttachmentLibraryMediaItem => {
  const primaryAttachment = item.attachments[0];
  const createdAt = toCreatedAt(primaryAttachment);
  const { fileTypeLabel } = getAttachmentMetadata(primaryAttachment);

  return {
    ...item,
    primaryAttachment,
    createdAt,
    createdLabel: formatCreatedAt(createdAt, locale),
    fileTypeLabel: item.kind === "motion" ? livePhotoLabel : fileTypeLabel,
    noteName: primaryAttachment.note,
  };
};

const groupMediaByMonth = (
  items: AttachmentLibraryMediaItem[],
  locale: string,
  unknownDateLabel: string,
): AttachmentLibraryMonthGroup[] => {
  const groups = new Map<string, AttachmentLibraryMediaItem[]>();

  for (const item of items) {
    const key = item.createdAt ? dayjs(item.createdAt).format("YYYY-MM") : "unknown";
    const group = groups.get(key) ?? [];
    group.push(item);
    groups.set(key, group);
  }

  return Array.from(groups.entries())
    .sort(([a], [b]) => (a === "unknown" ? 1 : b === "unknown" ? -1 : b.localeCompare(a)))
    .map(([key, groupedItems]) => ({
      key,
      label:
        key === "unknown"
          ? unknownDateLabel
          : dayjs(`${key}-01`).toDate().toLocaleDateString(locale, {
              month: "short",
              year: "numeric",
            }),
      items: groupedItems.sort((a, b) => sortByNewest(a.createdAt, b.createdAt)),
    }));
};

export function useAttachmentLibrary(locale: string, filter?: string) {
  const t = useTranslate();
  const query = useInfiniteAttachments({ ...ATTACHMENT_LIBRARY_REQUEST, filter });

  const attachments = useMemo(() => (query.data?.pages ?? []).flatMap((page) => page.attachments), [query.data?.pages]);

  const linkedAttachments = useMemo(
    () => attachments.filter(isLinkedAttachment).sort((a, b) => sortByNewest(toCreatedAt(a), toCreatedAt(b))),
    [attachments],
  );

  const mediaItems = useMemo(
    () =>
      buildAttachmentVisualItems(linkedAttachments.filter(isVisualAttachment))
        .map((item) => toLibraryMediaItem(item, locale, t("attachment-library.labels.live-photo")))
        .sort((a, b) => sortByNewest(a.createdAt, b.createdAt)),
    [linkedAttachments, locale, t],
  );

  const documentItems = useMemo(
    () =>
      linkedAttachments.filter((attachment) => !isVisualAttachment(attachment)).map((attachment) => toLibraryListItem(attachment, locale)),
    [linkedAttachments, locale],
  );

  const mediaGroups = useMemo(
    () => groupMediaByMonth(mediaItems, locale, t("attachment-library.labels.unknown-date")),
    [locale, mediaItems, t],
  );
  const mediaPreviewItems = useMemo(() => mediaItems.map((item) => item.previewItem), [mediaItems]);

  const stats = useMemo<AttachmentLibraryStats>(
    () => ({
      media: mediaItems.length,
      documents: documentItems.length,
    }),
    [documentItems.length, mediaItems.length],
  );

  return {
    ...query,
    attachments,
    mediaGroups,
    mediaItems,
    mediaPreviewItems,
    documentItems,
    stats,
  };
}

export function useAttachmentLibraryStats(filter?: string) {
  const query = useCompleteAttachments({ ...COMPLETE_ATTACHMENT_LIBRARY_REQUEST, filter });
  const attachments = useMemo(() => (query.data?.pages ?? []).flatMap((page) => page.attachments), [query.data?.pages]);
  const stats = useMemo(() => buildAttachmentLibraryStats(attachments), [attachments]);

  return { ...query, stats };
}

import dayjs from "dayjs";
import type { Element, Root as HtmlRoot } from "hast";
import type { Root, RootContent } from "mdast";
import { fromMarkdown } from "mdast-util-from-markdown";
import { gfmFromMarkdown } from "mdast-util-gfm";
import { gfm } from "micromark-extension-gfm";
import { defaultUrlTransform } from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkRehype from "remark-rehype";
import { unified } from "unified";
import { visit } from "unist-util-visit";
import { SANITIZE_SCHEMA } from "@/components/NoteContent/constants";
import type { NoteTimeBasis } from "@/contexts/ViewContext";
import { getNoteSortTime } from "@/hooks/useNoteSorting";
import { ISO_DATE_FORMAT } from "@/lib/calendar-utils";
import type { Note } from "@/types/proto/api/note_service_pb";
import { getAttachmentThumbnailUrl, getAttachmentUrl, isImage } from "@/utils/attachment";
import { classifyManagedAttachmentImageURL, extractAttachmentUIDFromName } from "@/utils/managed-attachment";

export interface CalendarDayExcerpt {
  noteName: string;
  /** The note's creator, so a mixed-author scope can attribute the preview. */
  creator: string;
  text: string;
  isCode: boolean;
}

export interface CalendarDayImage {
  noteName: string;
  thumbnailUrl: string;
}

export interface CalendarDaySummary {
  /** Every note, including hidden ones, in chronological order for the day panel. */
  notes: Note[];
  /** Each creator once, in the order they first wrote that day; a hidden note still names its author. */
  creators: string[];
  excerpt?: CalendarDayExcerpt;
  /** At most one image per note, from the first two eligible image-bearing notes. */
  images: CalendarDayImage[];
}

export type CalendarMonthModel = Record<string, CalendarDaySummary>;

export interface BuildCalendarMonthModelOptions {
  /** Hidden notes count, but never contribute text, filenames, or images. */
  isRedacted?: (note: Note) => boolean;
}

const plainText = (node: RootContent): string => {
  if (node.type === "text" || node.type === "inlineCode" || node.type === "code") return node.value;
  if (node.type === "break") return "\n";
  // Image alt text and link definitions are not a written excerpt. Never render raw HTML.
  if (!("children" in node)) return "";
  const separator = ["list", "listItem", "blockquote", "table", "tableRow"].includes(node.type) ? "\n" : "";
  return node.children.map((child) => plainText(child as RootContent)).join(separator);
};

const getExcerpt = (note: Note, tree: Root): CalendarDayExcerpt | undefined => {
  const blocks = tree.children;
  const readable = blocks.map((block) => ({ text: plainText(block).trim(), isCode: block.type === "code" })).filter(({ text }) => text);
  const text =
    readable
      .map((block) => block.text)
      .join("\n")
      .trim() || (!note.content.trim() ? note.snippet.trim() : "");
  if (!text) return undefined;
  return {
    noteName: note.name,
    creator: note.creator,
    text: text.length > 280 ? `${text.slice(0, 280).trimEnd()}…` : text,
    isCode: readable[0]?.isCode ?? false,
  };
};

// Resolve reference-style images and apply the same src sanitization as the body renderer.
// Raw HTML is deliberately excluded: only Markdown images contribute to this preview.
const imageProcessor = unified()
  .use(remarkRehype)
  .use([[rehypeSanitize, SANITIZE_SCHEMA]]);

const imageKey = (source: string): string => {
  try {
    return new URL(source, window.location.href).href;
  } catch {
    return source;
  }
};

const getImageCandidates = (note: Note, tree: Root): string[] => {
  const candidates: string[] = [];
  const attachments = note.attachments.filter((attachment) => isImage(attachment.type));
  const html = imageProcessor.runSync(tree) as HtmlRoot;
  visit(html, "element", (node: Element) => {
    if (node.tagName !== "img" || typeof node.properties.src !== "string") return;
    const source = defaultUrlTransform(node.properties.src);
    if (!source) return;
    const classified = classifyManagedAttachmentImageURL(source);
    if (classified.kind === "invalid") return;
    const attachment = attachments.find((candidate) =>
      classified.kind === "managed"
        ? extractAttachmentUIDFromName(candidate.name) === classified.uid
        : imageKey(getAttachmentUrl(candidate)) === imageKey(source),
    );
    // A managed image uses the attachment's thumbnail URL, just like an attached image.
    // Missing managed attachments must not be treated as external image URLs.
    if (classified.kind === "managed" && !attachment) return;
    candidates.push(attachment ? getAttachmentThumbnailUrl(attachment) : source);
  });
  candidates.push(...attachments.map(getAttachmentThumbnailUrl));
  return candidates;
};

const parseMarkdown = (content: string): Root => fromMarkdown(content, { extensions: [gfm()], mdastExtensions: [gfmFromMarkdown()] });

/**
 * The thumbnail of a note's first photo, chosen the way a calendar day chooses its photos:
 * Markdown images in reading order, then image attachments. Content with neither is not parsed.
 */
export const getNoteThumbnail = (note: Note): string | undefined => {
  if (!note.content.includes("![") && !note.attachments.some((attachment) => isImage(attachment.type))) return undefined;
  return getImageCandidates(note, parseMarkdown(note.content))[0];
};

/** Fold the month into stable day snapshots, independent of API pagination/order. */
export const buildCalendarMonthModel = (
  notes: Note[],
  timeBasis: NoteTimeBasis,
  { isRedacted }: BuildCalendarMonthModelOptions = {},
): CalendarMonthModel => {
  const dated = notes
    .map((note) => ({ note, time: getNoteSortTime(note, timeBasis) }))
    .filter((entry): entry is { note: Note; time: Date } => entry.time !== undefined)
    .sort((a, b) => a.time.getTime() - b.time.getTime() || a.note.name.localeCompare(b.note.name));

  const model: CalendarMonthModel = {};
  const fileFallbacks = new Map<string, CalendarDayExcerpt>();
  for (const { note, time } of dated) {
    const date = dayjs(time).format(ISO_DATE_FORMAT);
    const summary = (model[date] ??= { notes: [], creators: [], images: [] });
    summary.notes.push(note);
    if (!summary.creators.includes(note.creator)) summary.creators.push(note.creator);
    if (isRedacted?.(note)) continue;
    const tree = !summary.excerpt || summary.images.length < 2 ? parseMarkdown(note.content) : undefined;
    if (!summary.excerpt && tree) summary.excerpt = getExcerpt(note, tree);
    if (!fileFallbacks.has(date)) {
      const file = note.attachments.find((attachment) => !isImage(attachment.type) && !attachment.motionMedia?.groupId);
      if (file?.filename) fileFallbacks.set(date, { noteName: note.name, creator: note.creator, text: file.filename, isCode: false });
    }
    if (summary.images.length >= 2 || !tree) continue;
    const usedImages = new Set(summary.images.map((image) => imageKey(image.thumbnailUrl)));
    const thumbnailUrl = getImageCandidates(note, tree).find((source) => !usedImages.has(imageKey(source)));
    if (thumbnailUrl) summary.images.push({ noteName: note.name, thumbnailUrl });
  }
  for (const [date, summary] of Object.entries(model)) {
    summary.excerpt ??= fileFallbacks.get(date);
  }
  return model;
};

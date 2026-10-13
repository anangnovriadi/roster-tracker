import type React from "react";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";

export interface NoteContentProps {
  content: string;
  /** Attachments bound to this note, used to resolve managed image URLs in share/S3 modes. */
  attachments?: Attachment[];
  /** Resource name of the note (e.g. `notes/abc123`). Enables footnote links to target the note detail page. */
  noteName?: string;
  /** Collection page that rendered the note, preserved when an inline link opens its detail page. */
  parentPage?: string;
  /** The card renders collapsed (ClampedSection), so footnote links navigate instead of scrolling. */
  compact?: boolean;
  className?: string;
  contentClassName?: string;
  onClick?: (e: React.MouseEvent) => void;
  onDoubleClick?: (e: React.MouseEvent) => void;
}

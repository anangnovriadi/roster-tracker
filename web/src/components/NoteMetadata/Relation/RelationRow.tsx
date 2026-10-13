import { LinkIcon, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { createNoteNavigationState } from "@/components/NoteView/navigation";
import { buildNotePath } from "@/router/routes";
import type { NoteRelation_NoteRef } from "@/types/proto/api/note_service_pb";
import { METADATA_ROW_CLASSES, METADATA_ROW_LABEL_CLASSES, METADATA_ROW_TEXT_CLASSES, MetadataRowIconSlot } from "../MetadataSection";

interface RelationRowProps {
  note: NoteRelation_NoteRef;
  parentPage?: string;
  icon?: LucideIcon;
  /**
   * "row" is a complete row that is its own focus target; "label" is the focusable body
   * of a row whose box is a wrapper carrying other controls (the editor's remove button).
   */
  variant?: "row" | "label";
  /** Detail on the trailing rail, such as the relation's direction. */
  trailing?: ReactNode;
}

/** One related note as a metadata row: a glyph in the leading slot and the snippet. */
const RelationRow = ({ note, parentPage, icon = LinkIcon, variant = "row", trailing }: RelationRowProps) => (
  <Link
    className={variant === "row" ? METADATA_ROW_CLASSES : METADATA_ROW_LABEL_CLASSES}
    to={buildNotePath(note.name)}
    state={parentPage ? createNoteNavigationState(parentPage) : undefined}
    viewTransition
  >
    <MetadataRowIconSlot icon={icon} />
    <span className={METADATA_ROW_TEXT_CLASSES}>{note.snippet || <span className="italic text-muted-foreground/60">No content</span>}</span>
    {trailing}
  </Link>
);

export default RelationRow;

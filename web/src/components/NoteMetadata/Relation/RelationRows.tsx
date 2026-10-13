import { LinkIcon, MilestoneIcon } from "lucide-react";
import { useMemo } from "react";
import type { NoteRelation } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import { METADATA_ROW_DETAIL_CLASSES } from "../MetadataSection";
import RelationRow from "./RelationRow";
import { getRelationBuckets, getRelationNote, getRelationNoteName, type RelationDirection } from "./relationHelpers";
import { useResolvedRelationNotes } from "./useResolvedRelationNotes";

export interface RelationRowItem {
  relation: NoteRelation;
  direction: RelationDirection;
}

/** Every reference to or from the note, referencing first, as rows in one list. */
export const getRelationRowItems = (relations: NoteRelation[], currentNoteName?: string): RelationRowItem[] => {
  const { referencing, referenced } = getRelationBuckets(relations, currentNoteName);
  return [
    ...referencing.map((relation) => ({ relation, direction: "referencing" as const })),
    ...referenced.map((relation) => ({ relation, direction: "referenced" as const })),
  ];
};

const DIRECTION_ICONS = { referencing: LinkIcon, referenced: MilestoneIcon } as const;

interface RelationRowsProps {
  items: RelationRowItem[];
  parentPage?: string;
  /** Resolve fresh snippets only once the rows are near the viewport. */
  enabled: boolean;
}

/**
 * Related notes are rows like the note's files: a direction glyph in the slot, the
 * snippet, and the direction spelled out on the detail rail. No header or tabs, so the
 * list reads as part of the note rather than a panel attached to it.
 */
const RelationRows = ({ items, parentPage, enabled }: RelationRowsProps) => {
  const t = useTranslate();
  const noteNames = useMemo(
    () =>
      items.flatMap(({ relation, direction }) => {
        const note = getRelationNote(relation, direction);
        return note?.name ? [note.name] : [];
      }),
    [items],
  );
  const resolvedNotes = useResolvedRelationNotes(noteNames, { enabled });
  const directionLabels: Record<RelationDirection, string> = {
    referencing: t("common.referencing"),
    referenced: t("common.referenced-by"),
  };

  return (
    <>
      {items.map(({ relation, direction }) => {
        const note = getRelationNote(relation, direction);
        if (!note || resolvedNotes[note.name] === null) {
          return null;
        }
        return (
          <RelationRow
            key={getRelationNoteName(relation, direction)}
            note={resolvedNotes[note.name] ?? note}
            parentPage={parentPage}
            icon={DIRECTION_ICONS[direction]}
            trailing={<span className={METADATA_ROW_DETAIL_CLASSES}>{directionLabels[direction]}</span>}
          />
        );
      })}
    </>
  );
};

export default RelationRows;

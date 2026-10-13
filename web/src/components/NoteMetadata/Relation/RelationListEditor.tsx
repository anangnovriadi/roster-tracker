import type { FC } from "react";
import { useMemo } from "react";
import MetadataSection, { METADATA_ROW_BOX_CLASSES, MetadataRowRemoveControl } from "@/components/NoteMetadata/MetadataSection";
import type { NoteRelation } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import RelationRow from "./RelationRow";
import { getEditorReferenceRelations } from "./relationHelpers";
import { useResolvedRelationNotes } from "./useResolvedRelationNotes";

interface RelationListEditorProps {
  relations: NoteRelation[];
  onRelationsChange?: (relations: NoteRelation[]) => void;
  parentPage?: string;
  noteName?: string;
}

const RelationListEditor: FC<RelationListEditorProps> = ({ relations, onRelationsChange, parentPage, noteName }) => {
  const t = useTranslate();
  const referenceRelations = useMemo(() => getEditorReferenceRelations(relations, noteName), [relations, noteName]);
  const relatedNoteNames = useMemo(
    () => referenceRelations.flatMap((relation) => (relation.relatedNote?.name ? [relation.relatedNote.name] : [])),
    [referenceRelations],
  );
  const resolvedNotes = useResolvedRelationNotes(relatedNoteNames);

  const handleDeleteRelation = (noteName: string) => {
    if (onRelationsChange) {
      onRelationsChange(relations.filter((relation) => relation.relatedNote?.name !== noteName));
    }
  };

  if (referenceRelations.length === 0) {
    return null;
  }

  return (
    <MetadataSection title={t("common.relations")}>
      {referenceRelations.map((relation) => {
        const relatedNote = relation.relatedNote!;
        if (resolvedNotes[relatedNote.name] === null) return null;
        const note = relatedNote.snippet ? relatedNote : resolvedNotes[relatedNote.name] || relatedNote;
        // A split row: the link is the focusable body, and the remove control overlays the trailing end once the row is engaged.
        return (
          <div key={note.name} className={METADATA_ROW_BOX_CLASSES}>
            <RelationRow note={note} parentPage={parentPage} variant="label" />
            <MetadataRowRemoveControl label="Remove relation" onClick={() => handleDeleteRelation(note.name)} />
          </div>
        );
      })}
    </MetadataSection>
  );
};

export default RelationListEditor;

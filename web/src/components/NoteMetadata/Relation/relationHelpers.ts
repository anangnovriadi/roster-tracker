import type { NoteRelation } from "@/types/proto/api/note_service_pb";
import { NoteRelation_Type } from "@/types/proto/api/note_service_pb";

export type RelationDirection = "referencing" | "referenced";

export const isReferenceRelation = (relation: NoteRelation): boolean => relation.type === NoteRelation_Type.REFERENCE;

export const getEditorReferenceRelations = (relations: NoteRelation[], noteName?: string): NoteRelation[] => {
  return relations.filter(
    (relation) => isReferenceRelation(relation) && (!noteName || !relation.note?.name || relation.note.name === noteName),
  );
};

export const getRelationBuckets = (relations: NoteRelation[], currentNoteName?: string) => {
  return relations.reduce(
    (groups, relation) => {
      if (!isReferenceRelation(relation)) {
        return groups;
      }

      if (relation.note?.name === currentNoteName && relation.relatedNote?.name !== currentNoteName) {
        groups.referencing.push(relation);
      } else if (relation.note?.name !== currentNoteName && relation.relatedNote?.name === currentNoteName) {
        groups.referenced.push(relation);
      }

      return groups;
    },
    {
      referencing: [] as NoteRelation[],
      referenced: [] as NoteRelation[],
    },
  );
};

export const getRelationNote = (relation: NoteRelation, direction: RelationDirection) => {
  return direction === "referencing" ? relation.relatedNote : relation.note;
};

export const getRelationNoteName = (relation: NoteRelation, direction: RelationDirection): string => {
  return getRelationNote(relation, direction)?.name ?? "";
};

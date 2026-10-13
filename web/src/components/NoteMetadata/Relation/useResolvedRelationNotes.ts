import { create } from "@bufbuild/protobuf";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { findNoteInCollectionQueries, noteDetailQueryOptions } from "@/hooks/useNoteQueries";
import { type NoteRelation_NoteRef, NoteRelation_NoteRefSchema } from "@/types/proto/api/note_service_pb";

export const useResolvedRelationNotes = (noteNames: string[], options?: { enabled?: boolean }) => {
  const client = useQueryClient();
  const names = Array.from(new Set(noteNames.filter(Boolean)));
  const queries = useQueries({
    queries: names.map((name) => ({
      ...noteDetailQueryOptions(name),
      enabled: options?.enabled ?? true,
      initialData: () => findNoteInCollectionQueries(client, name, true),
    })),
  });
  const resolved: Record<string, NoteRelation_NoteRef | null> = {};
  queries.forEach((query, index) => {
    if (query.data === null) resolved[names[index]] = null;
    else if (query.data) {
      resolved[names[index]] = create(NoteRelation_NoteRefSchema, { name: query.data.name, snippet: query.data.snippet });
    }
  });
  return resolved;
};

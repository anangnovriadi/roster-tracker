import { useMemo } from "react";
import { useNoteFilterContext } from "@/contexts/NoteFilterContext";
import { getNoteSuggestions, type NoteSuggestion } from "@/lib/note-suggestions";
import { useSelectedNoteViewFilter } from "./useNoteFilters";

/** The same browsing context supplies both inline and full-screen composers. */
export function useNoteSuggestions(): NoteSuggestion[] {
  const { filters } = useNoteFilterContext();
  const viewFilter = useSelectedNoteViewFilter();
  return useMemo(() => getNoteSuggestions(filters, viewFilter), [filters, viewFilter]);
}

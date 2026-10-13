import { loadWithReload } from "@/utils/lazy";

type NoteEditorModule = typeof import(".");

let noteEditorModulePromise: Promise<NoteEditorModule> | undefined;

// Share one in-flight request across edit and comment entry points. Callers
// keep their current UI mounted until this promise resolves.
export const loadNoteEditor = (): Promise<NoteEditorModule> => {
  noteEditorModulePromise ??= loadWithReload(() => import(".")).catch((error) => {
    noteEditorModulePromise = undefined;
    throw error;
  });
  return noteEditorModulePromise;
};

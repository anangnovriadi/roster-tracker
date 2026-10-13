import type { Note } from "@/types/proto/api/note_service_pb";

/** How the header names the note's time: relative to now, or just the clock time for lists that already name the day. */
export type NoteTimeDisplay = "relative" | "time";

export interface NoteViewProps {
  note: Note;
  compact?: boolean;
  timeDisplay?: NoteTimeDisplay;
  showCreator?: boolean;
  showVisibility?: boolean;
  showPinned?: boolean;
  className?: string;
  parentPage?: string;
}

export interface NoteViewHandle {
  openEditor: () => void;
}

export interface NoteHeaderProps {
  timeDisplay?: NoteTimeDisplay;
  showCreator?: boolean;
  showVisibility?: boolean;
}

export interface NoteBodyProps {
  compact?: boolean;
}

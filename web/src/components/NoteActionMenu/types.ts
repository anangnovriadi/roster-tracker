import type { Note } from "@/types/proto/api/note_service_pb";

export interface NoteActionMenuProps {
  note: Note;
  parentPage?: string;
  readonly?: boolean;
  onEdit?: () => void;
}

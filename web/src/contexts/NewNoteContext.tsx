import { createContext, type Dispatch, type ReactNode, type SetStateAction, useContext, useMemo, useState } from "react";

interface NewNoteContextValue {
  // Name of the most recently created note in this view, or null. Only one
  // note is ever "new" at a time: creating another overwrites it, which clears
  // the previous one for free.
  newNoteName: string | null;
  markNewNote: Dispatch<SetStateAction<string | null>>;
}

// Default is a safe no-op so NoteEditor/NoteView rendered outside a provider
// (dialogs, detail pages, other lists) keep working without a "new" marker.
const NewNoteContext = createContext<NewNoteContextValue>({
  newNoteName: null,
  markNewNote: () => {},
});

export function NewNoteProvider({ children }: { children: ReactNode }) {
  const [newNoteName, setNewNoteName] = useState<string | null>(null);
  // setNewNoteName is identity-stable, so the value only changes with newNoteName.
  // Memoize it so a Home re-render doesn't cascade into every subscribed note row.
  const value = useMemo(() => ({ newNoteName, markNewNote: setNewNoteName }), [newNoteName]);

  return <NewNoteContext.Provider value={value}>{children}</NewNoteContext.Provider>;
}

export function useNewNote() {
  return useContext(NewNoteContext);
}

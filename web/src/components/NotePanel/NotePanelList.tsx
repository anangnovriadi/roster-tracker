import { PlusIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { MentionResolutionProvider } from "@/components/NoteContent/MentionResolutionContext";
import NoteEditor from "@/components/NoteEditor";
import type { NoteEditorProps } from "@/components/NoteEditor/types/components";
import NoteView from "@/components/NoteView";
import type { NoteTimeDisplay } from "@/components/NoteView/types";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/contexts/AuthContext";
import { NewNoteProvider } from "@/contexts/NewNoteContext";
import { useSpaceContext } from "@/contexts/SpaceContext";
import { useView } from "@/contexts/ViewContext";
import { cn } from "@/lib/utils";
import type { Note } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";

// The cards' own bottom margin sets the gap; the row's edge lines up with theirs.
const COMPOSE_ROW_CLASS = cn(buttonVariants({ variant: "quiet", size: "sm" }), "self-start");

export interface NotePanelCompose {
  cacheKey: string;
  /** The quiet row's label, e.g. "New note on this day". */
  label: string;
  /** What the new note inherits from the selection: its day, its place. */
  defaults?: Pick<NoteEditorProps, "defaultCreateTime" | "defaultLocation">;
  /** Whether the editor is open in place of the row, for hosts whose own gestures could dismiss it. */
  onComposingChange?: (composing: boolean) => void;
  onSavingChange?: (saving: boolean) => void;
  onConfirm?: (noteName: string) => void;
}

export interface NotePanelListProps {
  notes: Note[];
  /**
   * Identity of the selection the list shows. A change closes an open composer, so a
   * half-written note can never silently move to another day or place.
   */
  selectionKey: string;
  timeDisplay?: NoteTimeDisplay;
  /** Shown in place of the cards when there are none; omit to show nothing. */
  emptyText?: string;
  /** Present when a note can be appended to the selection. */
  compose?: NotePanelCompose;
}

/**
 * A selection's notes as the ordinary note cards, so editing, reactions and comments behave
 * exactly as on Home, ending with a quiet "new note" row that expands into the editor in place,
 * the way journals append at the end of a day.
 */
export function NotePanelList({ notes, selectionKey, timeDisplay, emptyText, compose }: NotePanelListProps) {
  const t = useTranslate();
  const { isUserSettingsInitialized } = useAuth();
  const { creatorUsername } = useSpaceContext();
  const { compactMode } = useView();
  const [composingFor, setComposingFor] = useState<string>();
  const composing = composingFor === selectionKey;
  const onComposingChange = compose?.onComposingChange;
  useEffect(() => {
    onComposingChange?.(composing);
    return () => onComposingChange?.(false);
  }, [composing, onComposingChange]);

  const contents = useMemo(() => notes.map((note) => note.content), [notes]);
  // A mixed-author selection shows each card's creator; resolve them with the reactors in one batch.
  const showCreator = !creatorUsername;
  const userNames = useMemo(
    () =>
      Array.from(
        new Set(notes.flatMap((note) => [...(showCreator ? [note.creator] : []), ...note.reactions.map((reaction) => reaction.creator)])),
      ),
    [notes, showCreator],
  );

  return (
    <NewNoteProvider>
      <MentionResolutionProvider contents={contents} userNames={userNames}>
        {notes.map((note) => (
          <NoteView key={note.name} note={note} timeDisplay={timeDisplay} showCreator={showCreator} showVisibility compact={compactMode} />
        ))}
      </MentionResolutionProvider>
      {emptyText && notes.length === 0 && <p className="text-sm text-muted-foreground">{emptyText}</p>}
      {compose &&
        isUserSettingsInitialized &&
        (composing ? (
          <NoteEditor
            cacheKey={compose.cacheKey}
            autoFocus
            placeholder={t("editor.any-thoughts")}
            {...compose.defaults}
            onSavingChange={compose.onSavingChange}
            onConfirm={(name) => {
              setComposingFor(undefined);
              compose.onConfirm?.(name);
            }}
            onCancel={() => setComposingFor(undefined)}
          />
        ) : (
          <button type="button" className={COMPOSE_ROW_CLASS} onClick={() => setComposingFor(selectionKey)}>
            <PlusIcon strokeWidth={1.8} />
            <span>{compose.label}</span>
          </button>
        ))}
    </NewNoteProvider>
  );
}

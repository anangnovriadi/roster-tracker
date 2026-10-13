import {
  type ComponentType,
  forwardRef,
  memo,
  Suspense,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { useLocation } from "react-router-dom";
import { useColumnGridUntrapped } from "@/components/ColumnGrid/ColumnGridContext";
import { useResolvedUser } from "@/components/NoteContent/MentionResolutionContext";
import { loadNoteEditor } from "@/components/NoteEditor/loader";
import type { NoteEditorProps } from "@/components/NoteEditor/types";
import { useAuth } from "@/contexts/AuthContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { isNoteBlurred } from "@/lib/tag";
import { cn } from "@/lib/utils";
import { State } from "@/types/proto/api/common_pb";
import { lazyWithReload } from "@/utils/lazy";
import { canManageNote } from "@/utils/user";
import { NoteBody, NoteCommentListView, NoteHeader } from "./components";
import NotePinnedMark from "./components/NotePinnedMark";
import { NOTE_CARD_BASE_CLASSES } from "./constants";
import { useImagePreview } from "./hooks";
import { computeCommentAmount, NoteViewContext } from "./NoteViewContext";
import { isNoteDetailPath, resolveNoteParentPage } from "./navigation";
import type { NoteViewHandle, NoteViewProps } from "./types";

const PreviewImageDialog = lazyWithReload(() => import("../PreviewImageDialog"));

const NoteView = forwardRef<NoteViewHandle, NoteViewProps>((props, ref) => {
  const { note: noteData, className, parentPage: parentPageProp, compact, timeDisplay, showCreator, showVisibility, showPinned } = props;
  const cardRef = useRef<HTMLDivElement>(null);
  const [showEditor, setShowEditor] = useState(false);
  const [EditorComponent, setEditorComponent] = useState<ComponentType<NoteEditorProps>>();

  const currentUser = useCurrentUser();
  const { userTagsSetting } = useAuth();
  const creator = useResolvedUser(noteData.creator, { enabled: Boolean(showCreator) });
  const isArchived = noteData.state === State.ARCHIVED;
  const readonly = !canManageNote(noteData, currentUser);
  const location = useLocation();
  const parentPage = resolveNoteParentPage({
    explicitParentPage: parentPageProp,
    pathname: location.pathname,
    search: location.search,
    noteName: noteData.name,
  });

  // Blur content when any tag has blur_content enabled in the current user's tag settings.
  const [showBlurredContent, setShowBlurredContent] = useState(false);
  const blurred = isNoteBlurred(noteData, userTagsSetting);
  const toggleBlurVisibility = useCallback(() => setShowBlurredContent((prev) => !prev), []);

  const { previewState, openPreview, setPreviewOpen } = useImagePreview();
  const editorHostRef = useRef<HTMLDivElement>(null);

  const focusMountedEditor = useCallback(() => {
    const codeMirrorContent = editorHostRef.current?.querySelector<HTMLElement>('.cm-content[contenteditable="true"]');
    const fallbackInput = editorHostRef.current?.querySelector<HTMLElement>("textarea, input");
    (codeMirrorContent ?? fallbackInput)?.focus();
  }, []);

  const openEditor = useCallback(() => {
    if (showEditor && EditorComponent) {
      focusMountedEditor();
      return;
    }
    void loadNoteEditor()
      .then(({ default: NoteEditor }) => {
        setEditorComponent(() => NoteEditor);
        setShowEditor(true);
      })
      .catch(() => undefined);
  }, [EditorComponent, focusMountedEditor, showEditor]);
  const closeEditor = useCallback(() => setShowEditor(false), []);

  // The grid keys tiles by note name (see getNoteKey), so the focused editor
  // identifies its own tile by name and untraps it for the duration.
  const { setUntrappedKey, clearUntrappedKey } = useColumnGridUntrapped();
  const handleFocusModeChange = useCallback(
    (isFocusMode: boolean) => {
      if (isFocusMode) {
        setUntrappedKey(noteData.name);
      } else {
        clearUntrappedKey(noteData.name);
      }
    },
    [noteData.name, setUntrappedKey, clearUntrappedKey],
  );
  // Release the slot when the editor closes or this card unmounts. Both calls
  // are ownership-scoped, so mounting cards never clobber another card's slot.
  useEffect(() => {
    if (!showEditor) {
      clearUntrappedKey(noteData.name);
    }
  }, [showEditor, noteData.name, clearUntrappedKey]);
  useEffect(() => () => clearUntrappedKey(noteData.name), [noteData.name, clearUntrappedKey]);

  useImperativeHandle(ref, () => ({ openEditor }), [openEditor]);

  const isInNoteDetailPage = isNoteDetailPath(location.pathname, noteData.name);
  const showCommentPreview = !isInNoteDetailPage && computeCommentAmount(noteData) > 0;

  const contextValue = useMemo(
    () => ({
      note: noteData,
      creator,
      currentUser,
      parentPage,
      isArchived,
      readonly,
      showBlurredContent,
      blurred,
      openEditor,
      toggleBlurVisibility,
      openPreview,
    }),
    [
      noteData,
      creator,
      currentUser,
      parentPage,
      isArchived,
      readonly,
      showBlurredContent,
      blurred,
      openEditor,
      toggleBlurVisibility,
      openPreview,
    ],
  );

  const article = (
    <article
      className={cn(NOTE_CARD_BASE_CLASSES, "group/note", showCommentPreview ? "mb-0 rounded-b-none" : "mb-2", className)}
      ref={cardRef}
      tabIndex={readonly ? -1 : 0}
    >
      {showPinned && noteData.pinned && <NotePinnedMark />}
      <NoteHeader timeDisplay={timeDisplay} showCreator={showCreator} showVisibility={showVisibility} />

      <NoteBody compact={compact} />

      {previewState.items.length > 0 && (
        <Suspense fallback={null}>
          <PreviewImageDialog
            open={previewState.open}
            onOpenChange={setPreviewOpen}
            items={previewState.items}
            initialIndex={previewState.index}
          />
        </Suspense>
      )}
    </article>
  );

  const noteDisplay = showCommentPreview ? (
    <div className="w-full mb-2">
      {article}
      <NoteCommentListView />
    </div>
  ) : (
    article
  );

  return (
    <NoteViewContext.Provider value={contextValue}>
      {showEditor && EditorComponent ? (
        <div ref={editorHostRef} className="w-full">
          <EditorComponent
            autoFocus
            className="mb-2"
            cacheKey={`inline-note-editor-${noteData.name}`}
            note={noteData}
            parentNoteName={noteData.parent || undefined}
            onConfirm={closeEditor}
            onCancel={closeEditor}
            onFocusModeChange={handleFocusModeChange}
          />
        </div>
      ) : (
        noteDisplay
      )}
    </NoteViewContext.Provider>
  );
});

NoteView.displayName = "NoteView";

export default memo(NoteView);

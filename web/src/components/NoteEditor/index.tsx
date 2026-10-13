import { useCallback, useEffect, useMemo, useRef } from "react";
import { useInstance } from "@/contexts/InstanceContext";
import { useLocalStorage } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { cn } from "@/lib/utils";
import { InstanceSetting_Key } from "@/types/proto/api/instance_service_pb";
import { Visibility } from "@/types/proto/api/note_service_pb";
import { canManageNote } from "@/utils/user";
import { EditorContent, EditorMetadata, FocusModeOverlay, TimestampPopover } from "./components";
import { EditorSuggestions } from "./components/EditorSuggestions";
import { FOCUS_MODE_STYLES, FORMATTING_TOOLBAR_STORAGE_KEY } from "./constants";
import type { EditorFileOrigin } from "./Editor/extensions";
import {
  splitInlineLocalFiles,
  toLocalFiles,
  useAttachmentTranscription,
  useAutoSave,
  useBlobUrls,
  useFocusMode,
  useInlineImageUpload,
  useNoteInit,
  useNoteSave,
} from "./hooks";
import { cacheService } from "./services";
import { EditorProvider, useEditorContext, useEditorSelector } from "./state";
import { EditorToolbar, FormattingToolbar } from "./Toolbar";
import type { EditorViewToggles, NoteEditorProps } from "./types";
import type { EditorController } from "./types/editorController";

// A host that presents the editor full-screen supplies `onFocusModeExit`; its
// presence is what makes an instance hosted, so focus mode starts on and stays on.
const NoteEditor = (props: NoteEditorProps) => (
  <EditorProvider initialFocusMode={Boolean(props.onFocusModeExit)}>
    <NoteEditorImpl {...props} />
  </EditorProvider>
);

const NoteEditorImpl: React.FC<NoteEditorProps> = ({
  className,
  cacheKey,
  note,
  parentNoteName,
  suggestions = [],
  defaultSpace,
  defaultLocation,
  autoFocus,
  onFocusModeExit,
  onFocusModeChange,
  placeholder,
  defaultCreateTime,
  onConfirm,
  onCancel,
  onSavingChange,
}) => {
  const currentUser = useCurrentUser();
  const editorRef = useRef<EditorController>(null);
  const { actions, dispatch, getState } = useEditorContext();
  // Subscribe only to the low-frequency slices this component renders from, so
  // typing (which changes content) does not re-render the editor shell and its
  // toolbar/metadata children.
  const isFocusMode = useEditorSelector((s) => s.ui.isFocusMode);
  // Report focus-mode changes so a host can react; inline hosts pass nothing.
  useEffect(() => {
    onFocusModeChange?.(isFocusMode);
  }, [isFocusMode, onFocusModeChange]);
  const isSaving = useEditorSelector((s) => s.ui.isLoading.saving);
  const hasTimestamp = useEditorSelector((s) => Boolean(s.timestamps.createTime));
  const { aiSetting, fetchSetting } = useInstance();
  const { createBlobUrl } = useBlobUrls();
  // Uploads keep their media metadata (camera, capture time, location); there is no opt-out.
  const saveMediaMetadata = true;
  const inlineImageUpload = useInlineImageUpload(editorRef);
  // Persisted preference: also show the formatting toolbar in normal mode. Focus
  // mode always shows it regardless; this only governs the non-focus layout.
  const [isFormattingToolbarVisible, setFormattingToolbarVisible] = useLocalStorage(FORMATTING_TOOLBAR_STORAGE_KEY, false);

  const noteName = note?.name;
  // Replies are never placed independently: a comment's Space is its parent's.
  // An edited top-level note can change its Space; a new one can unless its
  // host fixes the target (e.g. composing inside a Space).
  const draftSpace = useEditorSelector((s) => s.metadata.space);
  const canChooseSpace = note ? !note.parent && canManageNote(note, currentUser) : !parentNoteName && !defaultSpace;
  const editorSpace = note ? draftSpace : parentNoteName ? undefined : (defaultSpace ?? draftSpace);
  const canTranscribe = useMemo(() => {
    const providerId = aiSetting.transcription?.providerId ?? "";
    if (!providerId) return false;
    const provider = aiSetting.providers.find((p) => p.id === providerId);
    return Boolean(provider?.apiKeySet);
  }, [aiSetting.providers, aiSetting.transcription?.providerId]);

  // Get default visibility from user settings
  // Every note is public; there is no audience to choose.
  const defaultVisibility = Visibility.PUBLIC;
  const editorCacheKey = cacheService.key(currentUser?.name ?? "", cacheKey);

  const { isInitialized } = useNoteInit({
    editorRef,
    note,
    cacheKey,
    username: currentUser?.name ?? "",
    autoFocus,
    defaultVisibility,
    defaultCreateTime,
    defaultLocation,
    canChooseSpace,
  });
  const isDraftCacheEnabled = !note;

  useEffect(() => {
    onSavingChange?.(isSaving);
  }, [isSaving, onSavingChange]);
  // A composer that closes as its save completes unmounts before reporting the
  // save finished, so report it on the way out; a host must never stay locked.
  useEffect(() => () => onSavingChange?.(false), [onSavingChange]);

  // Auto-save content to localStorage (subscribes to the store internally).
  const { discardDraft } = useAutoSave(currentUser?.name ?? "", cacheKey, isInitialized && isDraftCacheEnabled);

  const { containerRef: editorContainerRef, placeholderHeight } = useFocusMode(isFocusMode);

  // Live-sync the draft's createTime/updateTime to the calendar-derived prop.
  // Only applies in create mode; edit mode owns its own timestamps. Runs after
  // initial mount (the seed value is set in useNoteInit), and again whenever
  // the prop changes — e.g., when the user picks a different calendar date
  // while the editor is open.
  useEffect(() => {
    if (note) return;
    if (!isInitialized) return;
    dispatch(
      actions.setTimestamps({
        createTime: defaultCreateTime,
        updateTime: defaultCreateTime,
      }),
    );
  }, [defaultCreateTime, note, isInitialized, actions, dispatch]);

  useEffect(() => {
    if (!currentUser) {
      return;
    }

    void fetchSetting(InstanceSetting_Key.AI).catch(() => undefined);
  }, [currentUser, fetchSetting]);

  const insertTranscribedText = useCallback((text: string) => {
    const editor = editorRef.current;
    if (!editor) {
      return;
    }
    editor.insertMarkdown(text);
    editor.scrollToCursor();
  }, []);

  const { transcribeAttachment, transcribingAttachment } = useAttachmentTranscription(insertTranscribedText);

  // Transcription inserts into the draft when it lands, so saving waits for it.
  useEffect(() => {
    dispatch(actions.setLoading("transcribing", Boolean(transcribingAttachment)));
  }, [transcribingAttachment, actions, dispatch]);

  const rememberCursor = useCallback(() => {
    const cursor = editorRef.current?.getCursor();
    if (cursor !== undefined) {
      cacheService.saveCursor(editorCacheKey, cursor);
    }
  }, [editorCacheKey]);

  // Hosted: focus mode is the host's frame, so leaving it dismisses the host.
  // Inline: focus mode is a view this editor owns and toggles in place.
  const handleToggleFocusMode = () => {
    if (onFocusModeExit) {
      rememberCursor();
      onFocusModeExit();
      return;
    }
    dispatch(actions.toggleFocusMode());
  };

  const handleCancel = useCallback(() => {
    rememberCursor();
    onCancel?.();
  }, [onCancel, rememberCursor]);

  const handleToggleFormattingToolbar = useCallback(() => {
    setFormattingToolbarVisible((visible) => !visible);
  }, [setFormattingToolbarVisible]);

  /**
   * Single ingest point for files the editor receives. Inline placement writes
   * images into the text (at `position`, else the caret) and attaches the rest;
   * otherwise everything joins the attachment list.
   */
  const handleFiles = useCallback(
    (files: File[], placement: { inline: false } | { inline: true; position?: number }) => {
      if (getState().ui.isLoading.saving) return;
      const localFiles = toLocalFiles(files, { createBlobUrl, saveMediaMetadata });
      const { inline, attachments } = placement.inline ? splitInlineLocalFiles(localFiles) : { inline: [], attachments: localFiles };
      attachments.forEach((file) => dispatch(actions.addLocalFile(file)));
      if (placement.inline) inlineImageUpload.insertLocalImages(inline, placement.position);
    },
    [actions, createBlobUrl, dispatch, getState, inlineImageUpload.insertLocalImages, saveMediaMetadata],
  );

  /** The ＋ menu's Insert image: inline at the caret. */
  const handleInsertImages = useCallback((files: File[]) => handleFiles(files, { inline: true }), [handleFiles]);

  /**
   * A drop inlines images where they landed. A paste carries no placement
   * gesture, so it attaches like the ＋ menu's upload; the attachment list's
   * own Insert action is how a pasted image becomes inline.
   */
  const handleEditorFiles = useCallback(
    (files: File[], origin: EditorFileOrigin) =>
      handleFiles(files, origin.source === "drop" ? { inline: true, position: origin.position } : { inline: false }),
    [handleFiles],
  );

  // The ＋ menu's view toggles only describe how an inline editor presents
  // itself, so a hosted editor offers neither: its host owns the frame, and
  // focus mode already forces the formatting toolbar on.
  const viewToggles: EditorViewToggles | undefined = onFocusModeExit
    ? undefined
    : {
        onToggleFocusMode: handleToggleFocusMode,
        isFormattingToolbarVisible,
        onToggleFormattingToolbar: handleToggleFormattingToolbar,
      };

  const handleSave = useNoteSave({
    noteName,
    parentNoteName,
    defaultSpace,
    defaultVisibility,
    defaultCreateTime,
    discardDraft,
    onConfirm,
    onCancel: onCancel ? handleCancel : undefined,
  });

  return (
    <>
      <FocusModeOverlay isActive={isFocusMode} onToggle={handleToggleFocusMode} />

      {/*
        Layout structure:
        - Uses justify-between to push content to top and bottom
        - In focus mode: becomes fixed with specific spacing, editor grows to fill space
        - In normal mode: stays relative with max-height constraint
      */}
      {isFocusMode && placeholderHeight > 0 && (
        <div aria-hidden className={cn("w-full", className)} style={{ height: placeholderHeight }} />
      )}

      <div
        ref={editorContainerRef}
        className={cn(
          "group relative w-full flex flex-col justify-between items-start bg-card px-4 py-3 rounded-lg border border-border/70 gap-2",
          FOCUS_MODE_STYLES.transition,
          isFocusMode && cn(FOCUS_MODE_STYLES.container.base, FOCUS_MODE_STYLES.container.spacing),
          !isFocusMode && className,
        )}
      >
        {/* Formatting toolbar. Always shown in focus mode (trailing the button
            that leaves the current frame — minimize inline, close when hosted);
            in normal mode it appears only when the user toggled it on via the
            insert menu. */}
        {(isFocusMode || isFormattingToolbarVisible) && (
          <FormattingToolbar
            controllerRef={editorRef}
            exit={isFocusMode ? { action: onFocusModeExit ? "close" : "minimize", onExit: handleToggleFocusMode } : undefined}
          />
        )}

        {(noteName || (!note && hasTimestamp)) && (
          <div className="flex h-6 w-full items-center">
            <TimestampPopover />
          </div>
        )}

        {/* Editor content grows to fill available space in focus mode */}
        <EditorContent ref={editorRef} placeholder={placeholder} onSubmit={handleSave} onFiles={handleEditorFiles} />

        {/* Metadata and toolbar grouped together at bottom */}
        <div className="w-full flex flex-col gap-2">
          <EditorMetadata
            noteName={noteName}
            uploadingLocalFileURLs={inlineImageUpload.uploadingLocalFileURLs}
            onInsertAttachments={inlineImageUpload.insertRemoteImages}
            onInsertLocalFiles={inlineImageUpload.insertLocalImages}
            onTranscribeAttachment={canTranscribe ? transcribeAttachment : undefined}
            transcribingAttachment={transcribingAttachment}
            transcriptionDisabled={isSaving}
          />
          {!note && !parentNoteName && isInitialized && suggestions.length > 0 && (
            <EditorSuggestions suggestions={suggestions} controllerRef={editorRef} space={editorSpace} />
          )}
          <EditorToolbar
            onSave={handleSave}
            onCancel={onCancel ? handleCancel : undefined}
            noteName={noteName}
            parentNoteName={parentNoteName}
            viewToggles={viewToggles}
            onInsertImages={handleInsertImages}
          />
        </div>
      </div>
    </>
  );
};

export default NoteEditor;

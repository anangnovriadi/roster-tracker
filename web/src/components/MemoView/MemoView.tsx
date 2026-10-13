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
import { useResolvedUser } from "@/components/MemoContent/MentionResolutionContext";
import { loadMemoEditor } from "@/components/MemoEditor/loader";
import type { MemoEditorProps } from "@/components/MemoEditor/types";
import { useAuth } from "@/contexts/AuthContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { isMemoBlurred } from "@/lib/tag";
import { cn } from "@/lib/utils";
import { State } from "@/types/proto/api/common_pb";
import { lazyWithReload } from "@/utils/lazy";
import { canManageMemo } from "@/utils/user";
import { MemoBody, MemoCommentListView, MemoHeader } from "./components";
import MemoPinnedMark from "./components/MemoPinnedMark";
import { MEMO_CARD_BASE_CLASSES } from "./constants";
import { useImagePreview } from "./hooks";
import { computeCommentAmount, MemoViewContext } from "./MemoViewContext";
import { isMemoDetailPath, resolveMemoParentPage } from "./navigation";
import type { MemoViewHandle, MemoViewProps } from "./types";

const PreviewImageDialog = lazyWithReload(() => import("../PreviewImageDialog"));

const MemoView = forwardRef<MemoViewHandle, MemoViewProps>((props, ref) => {
  const { memo: memoData, className, parentPage: parentPageProp, compact, timeDisplay, showCreator, showVisibility, showPinned } = props;
  const cardRef = useRef<HTMLDivElement>(null);
  const [showEditor, setShowEditor] = useState(false);
  const [EditorComponent, setEditorComponent] = useState<ComponentType<MemoEditorProps>>();

  const currentUser = useCurrentUser();
  const { userTagsSetting } = useAuth();
  const creator = useResolvedUser(memoData.creator, { enabled: Boolean(showCreator) });
  const isArchived = memoData.state === State.ARCHIVED;
  const readonly = !canManageMemo(memoData, currentUser);
  const location = useLocation();
  const parentPage = resolveMemoParentPage({
    explicitParentPage: parentPageProp,
    pathname: location.pathname,
    search: location.search,
    memoName: memoData.name,
  });

  // Blur content when any tag has blur_content enabled in the current user's tag settings.
  const [showBlurredContent, setShowBlurredContent] = useState(false);
  const blurred = isMemoBlurred(memoData, userTagsSetting);
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
    void loadMemoEditor()
      .then(({ default: MemoEditor }) => {
        setEditorComponent(() => MemoEditor);
        setShowEditor(true);
      })
      .catch(() => undefined);
  }, [EditorComponent, focusMountedEditor, showEditor]);
  const closeEditor = useCallback(() => setShowEditor(false), []);

  // The grid keys tiles by memo name (see getMemoKey), so the focused editor
  // identifies its own tile by name and untraps it for the duration.
  const { setUntrappedKey, clearUntrappedKey } = useColumnGridUntrapped();
  const handleFocusModeChange = useCallback(
    (isFocusMode: boolean) => {
      if (isFocusMode) {
        setUntrappedKey(memoData.name);
      } else {
        clearUntrappedKey(memoData.name);
      }
    },
    [memoData.name, setUntrappedKey, clearUntrappedKey],
  );
  // Release the slot when the editor closes or this card unmounts. Both calls
  // are ownership-scoped, so mounting cards never clobber another card's slot.
  useEffect(() => {
    if (!showEditor) {
      clearUntrappedKey(memoData.name);
    }
  }, [showEditor, memoData.name, clearUntrappedKey]);
  useEffect(() => () => clearUntrappedKey(memoData.name), [memoData.name, clearUntrappedKey]);

  useImperativeHandle(ref, () => ({ openEditor }), [openEditor]);

  const isInMemoDetailPage = isMemoDetailPath(location.pathname, memoData.name);
  const showCommentPreview = !isInMemoDetailPage && computeCommentAmount(memoData) > 0;

  const contextValue = useMemo(
    () => ({
      memo: memoData,
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
      memoData,
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
      className={cn(MEMO_CARD_BASE_CLASSES, "group/memo", showCommentPreview ? "mb-0 rounded-b-none" : "mb-2", className)}
      ref={cardRef}
      tabIndex={readonly ? -1 : 0}
    >
      {showPinned && memoData.pinned && <MemoPinnedMark />}
      <MemoHeader timeDisplay={timeDisplay} showCreator={showCreator} showVisibility={showVisibility} />

      <MemoBody compact={compact} />

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

  const memoDisplay = showCommentPreview ? (
    <div className="w-full mb-2">
      {article}
      <MemoCommentListView />
    </div>
  ) : (
    article
  );

  return (
    <MemoViewContext.Provider value={contextValue}>
      {showEditor && EditorComponent ? (
        <div ref={editorHostRef} className="w-full">
          <EditorComponent
            autoFocus
            className="mb-2"
            cacheKey={`inline-memo-editor-${memoData.name}`}
            memo={memoData}
            parentMemoName={memoData.parent || undefined}
            onConfirm={closeEditor}
            onCancel={closeEditor}
            onFocusModeChange={handleFocusModeChange}
          />
        </div>
      ) : (
        memoDisplay
      )}
    </MemoViewContext.Provider>
  );
});

MemoView.displayName = "MemoView";

export default memo(MemoView);

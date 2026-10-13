import { LoaderCircleIcon, MessageCircleIcon } from "lucide-react";
import { type ComponentType, forwardRef, useCallback, useImperativeHandle, useRef, useState } from "react";
import { loadNoteEditor } from "@/components/NoteEditor/loader";
import type { NoteEditorProps } from "@/components/NoteEditor/types";
import NoteView from "@/components/NoteView";
import { Button } from "@/components/ui/button";
import useCurrentUser from "@/hooks/useCurrentUser";
import { NOTE_COMMENTS_ANCHOR_ID } from "@/lib/note-comments";
import { extractNoteIdFromName } from "@/lib/resource-names";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";

interface Props {
  note: Note;
  comments: Note[];
  commentCount?: number;
  parentPage?: string;
  hasMoreComments?: boolean;
  isFetchingMoreComments?: boolean;
  onLoadMoreComments?: () => void;
}

export interface NoteCommentSectionHandle {
  openEditor: () => Promise<void>;
  scrollIntoView: () => void;
}

const NoteCommentSection = forwardRef<NoteCommentSectionHandle, Props>(
  ({ note, comments, commentCount = comments.length, parentPage, hasMoreComments, isFetchingMoreComments, onLoadMoreComments }, ref) => {
    const t = useTranslate();
    const currentUser = useCurrentUser();
    const [showEditor, setShowEditor] = useState(false);
    const [isEditorLoading, setIsEditorLoading] = useState(false);
    const [EditorComponent, setEditorComponent] = useState<ComponentType<NoteEditorProps>>();
    const sectionRef = useRef<HTMLDivElement>(null);

    const canCreateComment = !!currentUser && note.state === State.NORMAL;
    const showCreateButton = canCreateComment && !showEditor;

    const handleCommentCreated = async (_noteCommentName: string) => {
      setShowEditor(false);
    };

    const preloadEditor = useCallback(() => {
      void loadNoteEditor().catch(() => undefined);
    }, []);

    const focusMountedEditor = useCallback(() => {
      const codeMirrorContent = sectionRef.current?.querySelector<HTMLElement>('.cm-content[contenteditable="true"]');
      const fallbackInput = sectionRef.current?.querySelector<HTMLElement>("textarea, input");
      (codeMirrorContent ?? fallbackInput)?.focus();
    }, []);

    const openEditor = useCallback(async () => {
      if (!canCreateComment || isEditorLoading) {
        return;
      }
      if (showEditor && EditorComponent) {
        focusMountedEditor();
        return;
      }

      setIsEditorLoading(true);
      try {
        const { default: NoteEditor } = await loadNoteEditor();
        setEditorComponent(() => NoteEditor);
        setShowEditor(true);
      } catch {
        // Chunk failures are handled by loadWithReload; keep the current UI mounted.
      } finally {
        setIsEditorLoading(false);
      }
    }, [EditorComponent, canCreateComment, focusMountedEditor, isEditorLoading, showEditor]);

    const scrollIntoView = useCallback(() => {
      sectionRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
    }, []);

    useImperativeHandle(ref, () => ({ openEditor, scrollIntoView }), [openEditor, scrollIntoView]);

    return (
      <div ref={sectionRef} className="pt-8 pb-16 w-full" data-note-comments-section>
        <h2 id={NOTE_COMMENTS_ANCHOR_ID} className="sr-only">
          {t("note.comment.self")}
        </h2>
        <div className="relative mx-auto grow w-full min-h-full flex flex-col justify-start items-start gap-y-1">
          {comments.length === 0 ? (
            showCreateButton && (
              <div className="w-full flex flex-row justify-center items-center py-6">
                <Button
                  variant="ghost"
                  onPointerEnter={preloadEditor}
                  onFocus={preloadEditor}
                  onClick={openEditor}
                  disabled={isEditorLoading}
                >
                  <span className="text-muted-foreground">{t("note.comment.write-a-comment")}</span>
                  {isEditorLoading ? (
                    <LoaderCircleIcon className="ml-2 h-5 w-5 animate-spin text-muted-foreground" />
                  ) : (
                    <MessageCircleIcon className="ml-2 w-5 h-auto text-muted-foreground" />
                  )}
                </Button>
              </div>
            )
          ) : (
            <div className="w-full flex flex-row justify-between items-center h-8 pl-3 mb-2">
              <div className="flex flex-row justify-start items-center">
                <MessageCircleIcon className="w-5 h-auto text-muted-foreground mr-1" />
                <span className="text-muted-foreground text-sm">{t("note.comment.self")}</span>
                <span className="text-muted-foreground text-sm ml-1">({commentCount})</span>
              </div>
              {showCreateButton && (
                <Button
                  variant="ghost"
                  className="text-muted-foreground"
                  onPointerEnter={preloadEditor}
                  onFocus={preloadEditor}
                  onClick={openEditor}
                  disabled={isEditorLoading}
                >
                  {isEditorLoading && <LoaderCircleIcon className="h-4 w-4 animate-spin" />}
                  {t("note.comment.write-a-comment")}
                </Button>
              )}
            </div>
          )}
          {showEditor && EditorComponent && (
            <div className="w-full mb-2">
              <EditorComponent
                cacheKey={`${note.name}-comment`}
                placeholder={t("editor.add-your-comment-here")}
                parentNoteName={note.name}
                autoFocus
                onConfirm={handleCommentCreated}
                onCancel={() => setShowEditor(false)}
              />
            </div>
          )}
          {comments.map((comment) => (
            <div className="w-full" key={comment.name} id={extractNoteIdFromName(comment.name)}>
              <NoteView note={comment} parentPage={parentPage} showCreator compact />
            </div>
          ))}
          {hasMoreComments && (
            <div className="w-full mt-4 flex justify-center">
              <Button variant="outline" className="rounded-full px-4" onClick={onLoadMoreComments} disabled={isFetchingMoreComments}>
                {isFetchingMoreComments && <LoaderCircleIcon className="h-4 w-4 animate-spin" />}
                {t(isFetchingMoreComments ? "resource.fetching-data" : "note.load-more")}
              </Button>
            </div>
          )}
        </div>
      </div>
    );
  },
);

NoteCommentSection.displayName = "NoteCommentSection";

export default NoteCommentSection;

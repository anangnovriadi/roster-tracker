import { Code, ConnectError } from "@connectrpc/connect";
import { ArrowUpLeftFromCircleIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { Link, Navigate, useLocation, useParams } from "react-router-dom";
import NoteCommentSection, { type NoteCommentSectionHandle } from "@/components/NoteCommentSection";
import { MentionResolutionProvider } from "@/components/NoteContent/MentionResolutionContext";
import NoteParentPlaceholder, { type NoteParentStatus } from "@/components/NoteParentPlaceholder";
import NoteView, { type NoteViewHandle } from "@/components/NoteView";
import { computeCommentAmount } from "@/components/NoteView/NoteViewContext";
import { createNoteNavigationState, resolveNoteDetailOrigin, wantsCommentComposer } from "@/components/NoteView/navigation";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import useNoteDetailError from "@/hooks/useNoteDetailError";
import { useInfiniteNoteComments, useNote } from "@/hooks/useNoteQueries";
import { useSharedNote, withShareAttachmentLinks } from "@/hooks/useNoteShareQueries";
import { LEGACY_NOTE_COMMENTS_ANCHOR_ID, NOTE_COMMENTS_ANCHOR_ID } from "@/lib/note-comments";
import { noteNamePrefix } from "@/lib/resource-names";
import { buildNotePath, ROUTES } from "@/router/routes";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";
import { findNoteAnchorTarget } from "@/utils/markdown-manipulation";
import { canWriteNotes } from "@/utils/user";

const NoteSidebarRegistration = ({
  note,
  parentNote,
  parentStatus,
  onParentRetry,
  from,
  commentCount,
  readonly,
  onEdit,
  onCommentsOpen,
  onCommentCreate,
}: {
  note: Note;
  parentNote?: Note;
  parentStatus?: NoteParentStatus;
  onParentRetry?: () => void;
  from: string;
  commentCount?: number;
  readonly: boolean;
  onEdit: () => void;
  onCommentsOpen: () => void;
  onCommentCreate: () => void;
}) => {
  const { setNoteDetail } = useAppSidebar();

  useEffect(() => {
    setNoteDetail({
      note,
      parentNote,
      parentStatus,
      onParentRetry,
      from,
      commentCount,
      readonly,
      onEdit,
      onCommentsOpen,
      onCommentCreate,
    });
  }, [commentCount, from, note, onCommentCreate, onCommentsOpen, onEdit, parentNote, parentStatus, onParentRetry, readonly, setNoteDetail]);

  useEffect(() => () => setNoteDetail(undefined), [setNoteDetail]);

  return null;
};

const NoteDetail = () => {
  const { currentUser, isInitialized: authInitialized } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  const params = useParams();
  const location = useLocation();
  const { state: locationState, hash } = location;
  const noteViewRef = useRef<NoteViewHandle>(null);
  const commentSectionRef = useRef<NoteCommentSectionHandle>(null);
  const handleEdit = useCallback(() => noteViewRef.current?.openEditor(), []);

  // Detect share mode from the route parameter.
  const shareToken = params.token;
  const isShareMode = !!shareToken;

  // Primary note fetch — share token or direct name.
  const noteNameFromParams = params.uid ? `${noteNamePrefix}${params.uid}` : "";
  const {
    data: noteFromDirect,
    error: directError,
    isLoading: directLoading,
    isUnavailable: directUnavailable,
    fetchStatus: directFetchStatus,
  } = useNote(noteNameFromParams, { enabled: !isShareMode && !!noteNameFromParams });
  const { data: noteFromShare, error: shareError, isLoading: shareLoading } = useSharedNote(shareToken ?? "", { enabled: isShareMode });

  const note = isShareMode ? noteFromShare : noteFromDirect;
  const error = isShareMode ? shareError : directError;
  const isLoading = isShareMode ? shareLoading : directLoading;
  const hasExplicitOrigin =
    !!locationState && typeof locationState === "object" && typeof (locationState as { from?: unknown }).from === "string";
  const resolvedOrigin = resolveNoteDetailOrigin(locationState, { noteArchived: note?.state === State.ARCHIVED });
  const parentPage = !hasExplicitOrigin && !currentUser && note?.state !== State.ARCHIVED ? ROUTES.EXPLORE : resolvedOrigin;
  const noteName = note?.name ?? noteNameFromParams;
  const displayNote = useMemo(() => {
    if (!note) return undefined;
    if (!isShareMode) return note;
    return { ...note, attachments: withShareAttachmentLinks(note.attachments as Attachment[], shareToken!) };
  }, [isShareMode, note, shareToken]);

  useNoteDetailError({
    error: error as Error | null,
  });

  const {
    data: fetchedParent,
    error: parentError,
    isUnavailable: parentUnavailable,
    refetch: refetchParent,
  } = useNote(note?.parent || "", {
    enabled: !isShareMode && !!note?.parent,
  });

  // Private parents return 401 to guests. The transport still owns session
  // recovery; only a settled guest view treats that result as unavailable.
  const guestParentDenied =
    authInitialized && !currentUser && parentError instanceof ConnectError && parentError.code === Code.Unauthenticated;
  const parentStatus: NoteParentStatus | undefined =
    !isShareMode && note?.parent
      ? parentUnavailable || guestParentDenied
        ? "unavailable"
        : parentError
          ? "error"
          : !fetchedParent
            ? "loading"
            : undefined
      : undefined;
  const parentNote = parentStatus ? undefined : fetchedParent;
  const handleParentRetry = useCallback(() => {
    void refetchParent();
  }, [refetchParent]);

  const {
    data: comments = [],
    fetchNextPage: fetchNextComments,
    hasNextPage: hasNextComments,
    isFetchingNextPage: isFetchingNextComments,
  } = useInfiniteNoteComments(noteName, {
    enabled: !isShareMode && !!note,
  });
  const commentCount = note ? Math.max(computeCommentAmount(note), comments.length) : 0;

  const handleCommentsOpen = useCallback(() => {
    commentSectionRef.current?.scrollIntoView();
    window.history.replaceState(window.history.state, "", `${location.pathname}${location.search}#${NOTE_COMMENTS_ANCHOR_ID}`);
  }, [location.pathname, location.search]);

  const handleCommentCreate = useCallback(() => {
    handleCommentsOpen();
    void commentSectionRef.current?.openEditor();
  }, [handleCommentsOpen]);

  // A card's comment button lands here asking for the comment box; open it once per visit.
  const composedForRef = useRef("");
  const shouldCompose = wantsCommentComposer(locationState) && canWriteNotes(currentUser) && !isShareMode && note?.state === State.NORMAL;
  useEffect(() => {
    if (!shouldCompose || !note || composedForRef.current === location.key) return;
    if (!commentSectionRef.current) return;
    composedForRef.current = location.key;
    handleCommentCreate();
    // The comment section mounts after the note and its settings load, so re-check as they arrive.
  }, [shouldCompose, note, location.key, handleCommentCreate, comments, authInitialized, instanceInitialized]);

  // Scroll to the hash target once it's in the DOM. The effect re-runs as the note loads (footnote
  // anchors) and as comments arrive (comment anchors), since the target may render in either; the
  // ref guards against re-scrolling the same hash on every later comments page-load.
  const scrolledHashRef = useRef("");
  useEffect(() => {
    if (!hash) return;
    const scrollKey = `${noteName}\0${hash}`;
    if (scrolledHashRef.current === scrollKey) return;
    const fragment = decodeURIComponent(hash.slice(1));
    const commentSection = commentSectionRef.current;
    if (commentSection && (fragment === NOTE_COMMENTS_ANCHOR_ID || fragment === LEGACY_NOTE_COMMENTS_ANCHOR_ID)) {
      scrolledHashRef.current = scrollKey;
      commentSection.scrollIntoView();
      return;
    }

    // A legacy #comments link on a share-token page has no comment section. Let it
    // resolve to a historical body heading instead of turning into a dead fragment.
    const el = findNoteAnchorTarget(document, noteName, fragment);
    if (!el) return;
    scrolledHashRef.current = scrollKey;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [hash, note, noteName, comments]);

  // Keep the query mounted while revalidating a cached denial. Redirecting
  // earlier would abort the request that could confirm restored access.
  if (!isShareMode && directUnavailable && directFetchStatus === "idle" && !directError) return <Navigate to="/404" replace />;

  if (isShareMode) {
    const isNotFound = error instanceof ConnectError && (error.code === Code.NotFound || error.code === Code.Unauthenticated);
    if (isNotFound || (!isLoading && !note)) {
      return <Navigate to="/404" replace />;
    }
  }

  // Start the permitted requests as soon as routing is unlocked, but do not
  // expose content before tag-blur and instance display settings settle.
  if (isLoading || !note || !displayNote || !authInitialized || !instanceInitialized) {
    return null;
  }
  const mentionResolutionContents = [displayNote.content, ...comments.map((comment) => comment.content)];
  const userResolutionNames = Array.from(
    new Set([displayNote, ...comments].flatMap((item) => [item.creator, ...(item.reactions ?? []).map((reaction) => reaction.creator)])),
  );
  return (
    <section className="@container flex min-h-full w-full flex-col items-center pb-8 pt-3 md:pt-6">
      <MentionResolutionProvider contents={mentionResolutionContents} userNames={userResolutionNames}>
        <NoteSidebarRegistration
          note={displayNote}
          parentNote={parentNote}
          parentStatus={parentStatus}
          onParentRetry={handleParentRetry}
          from={parentPage}
          commentCount={isShareMode ? undefined : commentCount}
          readonly={isShareMode}
          onEdit={handleEdit}
          onCommentsOpen={handleCommentsOpen}
          onCommentCreate={handleCommentCreate}
        />
        <div className="w-full max-w-2xl px-4 sm:px-6">
          <div className="w-full">
            {!isShareMode && parentStatus && (
              <div className="mb-2 md:hidden">
                <NoteParentPlaceholder status={parentStatus} onRetry={handleParentRetry} />
              </div>
            )}
            {!isShareMode && parentNote && (
              <div className="w-auto inline-block mb-2 md:hidden">
                <Link
                  className="px-3 py-1 border border-border rounded-lg max-w-xs w-auto text-sm flex flex-row justify-start items-center flex-nowrap text-muted-foreground hover:shadow hover:opacity-80"
                  to={buildNotePath(parentNote.name)}
                  state={createNoteNavigationState(parentPage)}
                  viewTransition
                >
                  <ArrowUpLeftFromCircleIcon className="w-4 h-auto shrink-0 opacity-60 mr-2" />
                  <span className="truncate">{parentNote.content}</span>
                </Link>
              </div>
            )}
            <NoteView
              ref={noteViewRef}
              key={displayNote.name}
              note={displayNote}
              compact={false}
              parentPage={parentPage}
              showCreator
              showVisibility
            />
            {!isShareMode && (
              <NoteCommentSection
                ref={commentSectionRef}
                note={displayNote}
                comments={comments}
                commentCount={commentCount}
                parentPage={parentPage}
                hasMoreComments={hasNextComments}
                isFetchingMoreComments={isFetchingNextComments}
                onLoadMoreComments={fetchNextComments}
              />
            )}
          </div>
        </div>
      </MentionResolutionProvider>
    </section>
  );
};

export default NoteDetail;

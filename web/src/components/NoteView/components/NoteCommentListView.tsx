import { ChevronRightIcon } from "lucide-react";
import { Link } from "react-router-dom";
import MetadataSection, { METADATA_COMPACT_ROW_CLASSES, METADATA_ROW_SLOT_CLASSES } from "@/components/NoteMetadata/MetadataSection";
import { NotePreview } from "@/components/NotePreview";
import { FOCUS_VISIBLE_OUTLINE_CLASSES } from "@/components/ui/focus";
import { useNearViewport } from "@/hooks/useNearViewport";
import { useNoteComments } from "@/hooks/useNoteQueries";
import { useUsersByNames } from "@/hooks/useUserQueries";
import { NOTE_COMMENTS_ANCHOR_ID } from "@/lib/note-comments";
import { extractNoteIdFromName } from "@/lib/resource-names";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";
import UserAvatar from "../../UserAvatar";
import { useNoteViewContext, useNoteViewDerived } from "../NoteViewContext";
import { createNoteNavigationState } from "../navigation";

/**
 * "View all" speaks at the header's scale: 11px muted ink in a box as tall as the compact
 * header, so it sits on the title's line instead of stretching it.
 */
const VIEW_ALL_CLASSES = cn(
  "inline-flex h-5 items-center gap-0.5 rounded-md px-2 text-2xs text-muted-foreground/60 transition-colors hover:bg-muted/60 hover:text-foreground",
  FOCUS_VISIBLE_OUTLINE_CLASSES,
);

/**
 * The comment strip hangs off the card's bottom edge: a compact titled list of up to three
 * comments, each a 24px row whose avatar sits in the leading slot on the card's text edge.
 */
const NoteCommentListView: React.FC = () => {
  const t = useTranslate();
  const { note, parentPage } = useNoteViewContext();
  const { isInNoteDetailPage, commentAmount } = useNoteViewDerived();
  const { ref: viewportRef, isNearViewport } = useNearViewport<HTMLDivElement>();

  const { data } = useNoteComments(note.name, {
    enabled: isNearViewport && !isInNoteDetailPage && commentAmount > 0,
    pageSize: 3,
  });
  const comments = data?.comments ?? [];
  const displayedComments = comments.slice(0, 3);
  const { data: commentCreators } = useUsersByNames(displayedComments.map((comment) => comment.creator));

  if (isInNoteDetailPage || commentAmount === 0) {
    return null;
  }

  return (
    <div ref={viewportRef} className="rounded-b-lg border border-t-0 border-border/70 px-4 py-1">
      <MetadataSection
        title={t("note.comment.self")}
        count={commentAmount}
        density="compact"
        action={
          <Link
            to={`${buildNotePath(note.name)}#${NOTE_COMMENTS_ANCHOR_ID}`}
            state={createNoteNavigationState(parentPage)}
            className={VIEW_ALL_CLASSES}
          >
            {t("common.view-all")}
            <ChevronRightIcon className="-me-0.5 size-3" strokeWidth={2} />
          </Link>
        }
      >
        {displayedComments.map((comment) => {
          const uid = extractNoteIdFromName(comment.name);
          const creator = commentCreators?.get(comment.creator);
          return (
            <Link
              key={comment.name}
              to={`${buildNotePath(note.name)}#${uid}`}
              state={createNoteNavigationState(parentPage)}
              viewTransition
              className={METADATA_COMPACT_ROW_CLASSES}
            >
              <span className={METADATA_ROW_SLOT_CLASSES} aria-hidden="true">
                <UserAvatar className="size-4" avatarUrl={creator?.avatarUrl} name={creator?.displayName || creator?.username} />
              </span>
              <NotePreview
                className="min-w-0 flex-1"
                content={comment.snippet || comment.content}
                attachments={comment.attachments}
                creator={creator}
                showCreator
                truncate
              />
            </Link>
          );
        })}
      </MetadataSection>
    </div>
  );
};

export default NoteCommentListView;

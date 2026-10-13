import { CornerUpLeftIcon, Edit3Icon, LinkIcon, MessageCircleIcon, MessageSquarePlusIcon } from "lucide-react";
import { useMemo } from "react";
import { Link } from "react-router-dom";
import SidebarRow, { SIDEBAR_ROW_CLASSES, SIDEBAR_ROW_COUNT_RAIL_CLASSES, SidebarRowIconSlot } from "@/components/AppSidebar/SidebarRow";
import SidebarSection, { SIDEBAR_SECTION_STACK_CLASSES } from "@/components/AppSidebar/SidebarSection";
import { extractHeadings } from "@/components/NoteContent/pipeline";
import { getRelationBuckets, getRelationNote } from "@/components/NoteMetadata/Relation/relationHelpers";
import { useResolvedRelationNotes } from "@/components/NoteMetadata/Relation/useResolvedRelationNotes";
import NoteParentPlaceholder, { type NoteParentStatus } from "@/components/NoteParentPlaceholder";
import { createNoteNavigationState } from "@/components/NoteView/navigation";
import { useOverflowTitle } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { NOTE_COMMENTS_ANCHOR_ID } from "@/lib/note-comments";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import { State } from "@/types/proto/api/common_pb";
import { Note, type NoteRelation } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import { canManageNote, canWriteNotes } from "@/utils/user";
import NoteOutline from "./NoteOutline";

interface Props {
  note: Note;
  parentNote?: Note;
  parentStatus?: NoteParentStatus;
  onParentRetry?: () => void;
  parentPage?: string;
  commentCount?: number;
  className?: string;
  onEdit?: () => void;
  onCommentsOpen?: () => void;
  onCommentCreate?: () => void;
  onShareImageOpen?: () => void;
  forceReadonly?: boolean;
}

const normalizeSnippet = (value: string): string => value.replace(/\s+/g, " ").trim();

const BacklinkRow = ({
  relation,
  snippet,
  parentPage,
  referencedByLabel,
}: {
  relation: NoteRelation;
  snippet: string;
  parentPage?: string;
  referencedByLabel: string;
}) => {
  const { ref, title } = useOverflowTitle<HTMLSpanElement>(snippet);
  const relatedNote = getRelationNote(relation, "referenced");
  if (!relatedNote) {
    return null;
  }

  return (
    <Link
      aria-label={`${referencedByLabel}: ${snippet}`}
      className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
      to={buildNotePath(relatedNote.name)}
      state={parentPage ? createNoteNavigationState(parentPage) : undefined}
      title={title}
      viewTransition
    >
      <SidebarRowIconSlot icon={LinkIcon} />
      <span ref={ref} className="min-w-0 flex-1 truncate text-start">
        {snippet}
      </span>
    </Link>
  );
};

const NoteDetailSidebar = ({
  note,
  parentNote,
  parentStatus,
  onParentRetry,
  parentPage,
  commentCount,
  className,
  onEdit,
  onCommentsOpen,
  onCommentCreate,
  forceReadonly = false,
}: Props) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();

  const readonly = forceReadonly || !canManageNote(note, currentUser);
  const canEdit = !!onEdit && !readonly && note.state === State.NORMAL;
  const canComment = !!onCommentCreate && !forceReadonly && canWriteNotes(currentUser) && note.state === State.NORMAL;

  const headings = useMemo(() => extractHeadings(note.content), [note.content]);
  const { referenced } = useMemo(() => getRelationBuckets(note.relations, note.name), [note.relations, note.name]);
  const backlinkNoteNames = useMemo(
    () =>
      forceReadonly
        ? []
        : referenced.flatMap((relation) => {
            const relatedNote = getRelationNote(relation, "referenced");
            return relatedNote?.name ? [relatedNote.name] : [];
          }),
    [forceReadonly, referenced],
  );
  const resolvedNotes = useResolvedRelationNotes(backlinkNoteNames);

  const backlinkSnippet = (relation: NoteRelation) => {
    const relatedNote = getRelationNote(relation, "referenced");
    if (!relatedNote) {
      return "";
    }
    return normalizeSnippet(relatedNote.snippet || resolvedNotes[relatedNote.name]?.snippet || relatedNote.name);
  };

  const parentSnippet = parentNote ? normalizeSnippet(parentNote.snippet || parentNote.content || parentNote.name) : "";
  const showComments = !forceReadonly && commentCount !== undefined && commentCount > 0;
  const showOnThisNote = headings.length > 1 || showComments;
  const showConnections = !forceReadonly && (!!parentNote || !!parentStatus || referenced.length > 0);

  return (
    <div className={cn("relative w-full select-none", SIDEBAR_SECTION_STACK_CLASSES, className)}>
      {showOnThisNote && (
        <SidebarSection label={t("note.on-this-note")}>
          {headings.length > 1 && <NoteOutline headings={headings} noteName={note.name} />}
          {showComments && (
            <a
              className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
              href={`#${NOTE_COMMENTS_ANCHOR_ID}`}
              onClick={(event) => {
                if (!onCommentsOpen) return;
                event.preventDefault();
                onCommentsOpen();
              }}
            >
              <SidebarRowIconSlot icon={MessageCircleIcon} />
              <span className="min-w-0 flex-1 truncate text-start">{t("note.comment.self")}</span>
              <span className={SIDEBAR_ROW_COUNT_RAIL_CLASSES}>{commentCount}</span>
            </a>
          )}
        </SidebarSection>
      )}

      {showConnections && (
        <SidebarSection label={t("note.connections")}>
          {parentStatus && <NoteParentPlaceholder status={parentStatus} onRetry={onParentRetry} />}
          {!parentStatus && parentNote && (
            <Link
              aria-label={`${t("note.parent-note")}: ${parentSnippet}`}
              className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
              to={buildNotePath(parentNote.name)}
              state={parentPage ? createNoteNavigationState(parentPage) : undefined}
              title={parentSnippet}
              viewTransition
            >
              <SidebarRowIconSlot icon={CornerUpLeftIcon} />
              <span className="min-w-0 flex-1 truncate text-start">{parentSnippet}</span>
            </Link>
          )}
          {referenced.map((relation) => {
            const relatedNote = getRelationNote(relation, "referenced");
            if (relatedNote && resolvedNotes[relatedNote.name] === null) return null;
            return (
              <BacklinkRow
                key={`referenced-${relatedNote?.name}`}
                relation={relation}
                snippet={backlinkSnippet(relation)}
                parentPage={parentPage}
                referencedByLabel={t("common.referenced-by")}
              />
            );
          })}
        </SidebarSection>
      )}

      {(canEdit || canComment) && (
        <SidebarSection label={t("common.actions")}>
          {canEdit && <SidebarRow icon={Edit3Icon} label={t("common.edit")} onClick={onEdit} />}
          {canComment && <SidebarRow icon={MessageSquarePlusIcon} label={t("note.comment.write-a-comment")} onClick={onCommentCreate} />}
        </SidebarSection>
      )}
    </div>
  );
};

export default NoteDetailSidebar;

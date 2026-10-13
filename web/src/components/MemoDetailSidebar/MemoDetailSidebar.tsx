import { CornerUpLeftIcon, Edit3Icon, LinkIcon, MessageCircleIcon, MessageSquarePlusIcon } from "lucide-react";
import { useMemo } from "react";
import { Link } from "react-router-dom";
import SidebarRow, { SIDEBAR_ROW_CLASSES, SIDEBAR_ROW_COUNT_RAIL_CLASSES, SidebarRowIconSlot } from "@/components/AppSidebar/SidebarRow";
import SidebarSection, { SIDEBAR_SECTION_STACK_CLASSES } from "@/components/AppSidebar/SidebarSection";
import { extractHeadings } from "@/components/MemoContent/pipeline";
import { getRelationBuckets, getRelationMemo } from "@/components/MemoMetadata/Relation/relationHelpers";
import { useResolvedRelationMemos } from "@/components/MemoMetadata/Relation/useResolvedRelationMemos";
import MemoParentPlaceholder, { type MemoParentStatus } from "@/components/MemoParentPlaceholder";
import { createMemoNavigationState } from "@/components/MemoView/navigation";
import { useOverflowTitle } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { MEMO_COMMENTS_ANCHOR_ID } from "@/lib/memo-comments";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import { State } from "@/types/proto/api/common_pb";
import { Memo, type MemoRelation } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { canManageMemo } from "@/utils/user";
import MemoOutline from "./MemoOutline";

interface Props {
  memo: Memo;
  parentMemo?: Memo;
  parentStatus?: MemoParentStatus;
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
  relation: MemoRelation;
  snippet: string;
  parentPage?: string;
  referencedByLabel: string;
}) => {
  const { ref, title } = useOverflowTitle<HTMLSpanElement>(snippet);
  const relatedMemo = getRelationMemo(relation, "referenced");
  if (!relatedMemo) {
    return null;
  }

  return (
    <Link
      aria-label={`${referencedByLabel}: ${snippet}`}
      className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
      to={buildNotePath(relatedMemo.name)}
      state={parentPage ? createMemoNavigationState(parentPage) : undefined}
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

const MemoDetailSidebar = ({
  memo,
  parentMemo,
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

  const readonly = forceReadonly || !canManageMemo(memo, currentUser);
  const canEdit = !!onEdit && !readonly && memo.state === State.NORMAL;
  const canComment = !!onCommentCreate && !forceReadonly && !!currentUser && memo.state === State.NORMAL;

  const headings = useMemo(() => extractHeadings(memo.content), [memo.content]);
  const { referenced } = useMemo(() => getRelationBuckets(memo.relations, memo.name), [memo.relations, memo.name]);
  const backlinkMemoNames = useMemo(
    () =>
      forceReadonly
        ? []
        : referenced.flatMap((relation) => {
            const relatedMemo = getRelationMemo(relation, "referenced");
            return relatedMemo?.name ? [relatedMemo.name] : [];
          }),
    [forceReadonly, referenced],
  );
  const resolvedMemos = useResolvedRelationMemos(backlinkMemoNames);

  const backlinkSnippet = (relation: MemoRelation) => {
    const relatedMemo = getRelationMemo(relation, "referenced");
    if (!relatedMemo) {
      return "";
    }
    return normalizeSnippet(relatedMemo.snippet || resolvedMemos[relatedMemo.name]?.snippet || relatedMemo.name);
  };

  const parentSnippet = parentMemo ? normalizeSnippet(parentMemo.snippet || parentMemo.content || parentMemo.name) : "";
  const showComments = !forceReadonly && commentCount !== undefined && commentCount > 0;
  const showOnThisMemo = headings.length > 1 || showComments;
  const showConnections = !forceReadonly && (!!parentMemo || !!parentStatus || referenced.length > 0);

  return (
    <div className={cn("relative w-full select-none", SIDEBAR_SECTION_STACK_CLASSES, className)}>
      {showOnThisMemo && (
        <SidebarSection label={t("memo.on-this-memo")}>
          {headings.length > 1 && <MemoOutline headings={headings} memoName={memo.name} />}
          {showComments && (
            <a
              className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
              href={`#${MEMO_COMMENTS_ANCHOR_ID}`}
              onClick={(event) => {
                if (!onCommentsOpen) return;
                event.preventDefault();
                onCommentsOpen();
              }}
            >
              <SidebarRowIconSlot icon={MessageCircleIcon} />
              <span className="min-w-0 flex-1 truncate text-start">{t("memo.comment.self")}</span>
              <span className={SIDEBAR_ROW_COUNT_RAIL_CLASSES}>{commentCount}</span>
            </a>
          )}
        </SidebarSection>
      )}

      {showConnections && (
        <SidebarSection label={t("memo.connections")}>
          {parentStatus && <MemoParentPlaceholder status={parentStatus} onRetry={onParentRetry} />}
          {!parentStatus && parentMemo && (
            <Link
              aria-label={`${t("memo.parent-memo")}: ${parentSnippet}`}
              className={cn(SIDEBAR_ROW_CLASSES, "text-muted-foreground hover:bg-sidebar-accent/65 hover:text-foreground")}
              to={buildNotePath(parentMemo.name)}
              state={parentPage ? createMemoNavigationState(parentPage) : undefined}
              title={parentSnippet}
              viewTransition
            >
              <SidebarRowIconSlot icon={CornerUpLeftIcon} />
              <span className="min-w-0 flex-1 truncate text-start">{parentSnippet}</span>
            </Link>
          )}
          {referenced.map((relation) => {
            const relatedMemo = getRelationMemo(relation, "referenced");
            if (relatedMemo && resolvedMemos[relatedMemo.name] === null) return null;
            return (
              <BacklinkRow
                key={`referenced-${relatedMemo?.name}`}
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
          {canComment && <SidebarRow icon={MessageSquarePlusIcon} label={t("memo.comment.write-a-comment")} onClick={onCommentCreate} />}
        </SidebarSection>
      )}
    </div>
  );
};

export default MemoDetailSidebar;

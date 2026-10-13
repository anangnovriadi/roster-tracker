import { Link, useLocation } from "react-router-dom";
import RelativeTime from "@/components/RelativeTime";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useNewNote } from "@/contexts/NewNoteContext";
import i18n from "@/i18n";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import type { User } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import NoteActionMenu from "../../NoteActionMenu";
import { ReactionSelector } from "../../NoteReactionListView";
import { hasReactedToNote } from "../../NoteReactionListView/hooks";
import UserAvatar from "../../UserAvatar";
import { NOTE_TIME_CONTROL_CLASSES } from "../constants";
import { useNoteViewContext, useNoteViewDerived } from "../NoteViewContext";
import { createNoteNavigationState, isNoteDetailPath } from "../navigation";
import type { NoteHeaderProps } from "../types";

const NoteHeader: React.FC<NoteHeaderProps> = ({ timeDisplay = "relative", showCreator }) => {
  const t = useTranslate();

  const { note, creator, currentUser, parentPage, isArchived, readonly, openEditor } = useNoteViewContext();
  const location = useLocation();
  const { createTime, updateTime, displayTime: noteDisplayTime, isDisplayingUpdatedTime, relativeTimeFormat } = useNoteViewDerived();
  const { newNoteName } = useNewNote();

  const timeValue = isArchived ? (
    noteDisplayTime?.toLocaleString(i18n.language)
  ) : timeDisplay === "time" ? (
    noteDisplayTime?.toLocaleTimeString(i18n.language, { hour: "numeric", minute: "2-digit" })
  ) : (
    <RelativeTime date={noteDisplayTime} format={relativeTimeFormat} />
  );
  const displayTime = isDisplayingUpdatedTime ? (
    <>
      {t("common.last-updated-at")} {timeValue}
    </>
  ) : (
    timeValue
  );
  const timeTooltip = {
    createdAt: createTime ? `${t("common.created-at")}: ${createTime.toLocaleString(i18n.language)}` : undefined,
    updatedAt:
      updateTime && (!createTime || updateTime.getTime() !== createTime.getTime())
        ? `${t("common.last-updated-at")}: ${updateTime.toLocaleString(i18n.language)}`
        : undefined,
  };

  return (
    // A fixed 24px row, the height of its action squares, so the card's top edge never
    // moves with what the header happens to show.
    <div className="flex h-6 w-full items-center justify-between gap-2">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {/* The time stays visible while the creator and Space badge can shrink and truncate. */}
        <div data-slot="note-header-meta" className="flex min-w-0 items-center gap-1.5 overflow-hidden">
          {showCreator && creator && <CreatorDisplay creator={creator} />}
          <TimeDisplay
            displayTime={displayTime}
            timeTooltip={timeTooltip}
            noteName={note.name}
            parentPage={parentPage}
            isCurrentPage={isNoteDetailPath(location.pathname, note.name)}
          />
        </div>
        {note.name === newNoteName && (
          <span className="shrink-0 rounded-full bg-primary/10 px-1.5 py-0.5 text-2xs font-medium leading-none text-primary">
            {t("note.new-badge")}
          </span>
        )}
      </div>

      <div data-slot="note-header-actions" className="flex shrink-0 select-none flex-row items-center justify-end gap-1">
        {currentUser && !isArchived && !hasReactedToNote(note, currentUser.name) && (
          // On desktop the picker's trigger shows only while the card is engaged or the picker is open.
          <span className="flex sm:hidden sm:group-hover:flex sm:group-focus-within:flex sm:has-[[data-popup-open]]:flex">
            <ReactionSelector note={note} trigger={<Button variant="quiet" size="icon-sm" />} />
          </span>
        )}

        <NoteActionMenu note={note} parentPage={parentPage} readonly={readonly} onEdit={openEditor} />
      </div>
    </div>
  );
};

/**
 * The author on one line with the time: a 20px avatar in a 20px slot, then the name in
 * medium 13px foreground ink. It is the identity, so it carries the weight; the time after
 * it is muted.
 */
const CreatorDisplay: React.FC<{ creator: User }> = ({ creator }) => (
  <>
    {/* The author is named, not linked: notes are not browsed per member. */}
    <span className="flex min-w-0 shrink items-center gap-1.5 text-ui font-medium text-foreground">
      <span className="flex size-5 shrink-0 items-center justify-center">
        <UserAvatar className="size-5" avatarUrl={creator.avatarUrl} name={creator.displayName || creator.username} />
      </span>
      <span className="min-w-0 truncate">{creator.displayName || creator.username}</span>
    </span>
    <span aria-hidden="true" className="shrink-0 text-muted-foreground/40">
      ·
    </span>
  </>
);

interface TimeTooltipContent {
  createdAt?: string;
  updatedAt?: string;
}

const TimeTooltip = ({ children, content }: { children: React.ReactElement; content: TimeTooltipContent }) => (
  <Tooltip>
    <TooltipTrigger render={children} />
    <TooltipContent align="start" className="flex flex-col items-start gap-0.5 whitespace-nowrap text-left">
      {content.createdAt && <span>{content.createdAt}</span>}
      {content.updatedAt && <span>{content.updatedAt}</span>}
    </TooltipContent>
  </Tooltip>
);

interface TimeDisplayProps {
  displayTime: React.ReactNode;
  timeTooltip: TimeTooltipContent;
  noteName: string;
  parentPage: string;
  /** On the note's own page the permalink would lead nowhere, so the time is plain text. */
  isCurrentPage: boolean;
}

/** The timestamp is the note's permalink: a real link, so it previews its URL and opens in a new tab. */
const TimeDisplay: React.FC<TimeDisplayProps> = ({ displayTime, timeTooltip, noteName, parentPage, isCurrentPage }) => (
  <TimeTooltip content={timeTooltip}>
    {isCurrentPage ? (
      <span className="shrink-0 whitespace-nowrap text-ui text-muted-foreground select-none">{displayTime}</span>
    ) : (
      <Link
        className={cn(NOTE_TIME_CONTROL_CLASSES, "underline-offset-2 hover:underline")}
        to={buildNotePath(noteName)}
        state={createNoteNavigationState(parentPage)}
        viewTransition
      >
        {displayTime}
      </Link>
    )}
  </TimeTooltip>
);

export default NoteHeader;

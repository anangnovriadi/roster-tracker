import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { MentionResolutionProvider } from "@/components/NoteContent/MentionResolutionContext";
import { deriveDefaultCreateTimeFromDate } from "@/components/NoteEditor/utils/deriveDefaultCreateTime";
import { NOTE_PANEL_INSET, NOTE_PANEL_TITLE_CLASS, NOTE_PANEL_WIDTH_CSS, NotePanel, NotePanelList } from "@/components/NotePanel";
import NoteListError from "@/components/PagedNoteList/NoteListError";
import RosterTodayBanner from "@/components/RosterTodayBanner";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import { useSpaceContext } from "@/contexts/SpaceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useFilteredNoteStats } from "@/hooks/useFilteredNoteStats";
import useMediaQuery from "@/hooks/useMediaQuery";
import { useNoteFilters } from "@/hooks/useNoteFilters";
import { formatMonthLabel, getToday, parseLocalDate } from "@/lib/calendar-utils";
import { combineCELFilters } from "@/lib/cel-filter";
import { isNoteBlurred } from "@/lib/tag";
import type { Note } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import { PREVIEW_MIN_CELL_WIDTH } from "./CalendarDayCell";
import { CalendarGrid } from "./CalendarGrid";
import { CalendarHeader } from "./CalendarHeader";
import { buildCalendarPath, getDefaultDate } from "./paths";
import RosterDayPanel from "./RosterDayPanel";
import { useMonthNotes } from "./useMonthNotes";

const NO_NOTES: Note[] = [];
const NO_CONTENTS: string[] = [];
/**
 * What the pushed grid keeps beside the panel: page padding, the grid's 2px border and seven
 * preview-wide columns. The panel already subtracts both of its insets but the push leaves only
 * one as a gap, so one comes back off. A persisted card wider than this yields for as long as
 * the window is narrow, down to its minimum.
 */
export const RESERVED_BESIDE_PANEL = 48 + 2 + 7 * PREVIEW_MIN_CELL_WIDTH - NOTE_PANEL_INSET;

export interface CalendarViewProps {
  /** `YYYY-MM` */
  month: string;
  /** `YYYY-MM-DD` of the open day, if any. */
  date?: string;
}

/**
 * The readable notes in the selected creator and Space scope, shown as a month.
 * Month and day both live in the URL; this component only reads them and renders.
 *
 * The open day has two homes. From md it is the floating note panel at the end edge; at xl
 * the grid gives way to it and the card narrows so every column keeps its preview, below xl
 * it floats over the trailing columns. Below md the grid is compact and the day's list sits
 * under it, the way phone calendars work, so a day is always shown there: today in the
 * current month, otherwise the first of the month, until the URL names one.
 */
export const CalendarView = ({ month, date }: CalendarViewProps) => {
  const t = useTranslate();
  const { i18n } = useTranslation();
  const navigate = useNavigate();
  const { search } = useLocation();
  const user = useCurrentUser();
  const md = useMediaQuery("md");
  const xl = useMediaQuery("xl");
  const { userTagsSetting, isInitialized: authInitialized, isUserSettingsInitialized } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  const { noteFilter: contextFilter, creatorUsername } = useSpaceContext();

  // The sidebar's view and tag filters narrow the month exactly as they narrow Home. They are
  // not echoed as chips here: the sidebar already shows them checked and clears them on a
  // second click, and search hands off to Home, so nothing can be active without a sidebar row.
  const viewFilter = useNoteFilters({ includeNoteViews: true, includePinned: false });
  // The scope filter includes both creator and Space, and matches the sidebar statistics.
  const statsFilter = useMemo(() => combineCELFilters(contextFilter, viewFilter), [contextFilter, viewFilter]);
  const monthFilter = useMemo(() => combineCELFilters(contextFilter, viewFilter), [contextFilter, viewFilter]);

  // Statistics draw counts before the month's notes load, and are the only signal the grid
  // has for days outside the month.
  const { statistics } = useFilteredNoteStats({
    context: "collection",
    filter: statsFilter,
    enabled: authInitialized && instanceInitialized,
  });

  const isRedacted = useCallback((note: Note) => isNoteBlurred(note, userTagsSetting), [userTagsSetting]);
  // Snippets and thumbnails must not appear before the tag settings that decide what to blur
  // have loaded; until then the predicate would let everything through.
  const { model, isLoading, error, refetch } = useMonthNotes({
    month,
    filter: monthFilter,
    isRedacted,
    enabled: isUserSettingsInitialized,
  });

  const monthLabel = useMemo(() => formatMonthLabel(month, i18n.language), [month, i18n.language]);
  const closeDay = useCallback(() => navigate({ pathname: buildCalendarPath(month), search }), [navigate, month, search]);

  const today = getToday();
  const activeDate = date ?? (md ? undefined : getDefaultDate(month, today));
  const activeNotes = (activeDate && model[activeDate]?.notes) || NO_NOTES;
  const dateLabel = useMemo(
    () =>
      activeDate
        ? (parseLocalDate(activeDate)?.toLocaleDateString(i18n.language, { weekday: "long", month: "long", day: "numeric" }) ?? activeDate)
        : "",
    [activeDate, i18n.language],
  );
  const defaultCreateTime = useMemo(() => (activeDate ? deriveDefaultCreateTimeFromDate(activeDate) : undefined), [activeDate]);
  const [saving, setSaving] = useState(false);
  const panelOpen = Boolean(date) && md;

  const showAuthors = !creatorUsername;
  const authorNames = useMemo(
    () => (showAuthors ? Array.from(new Set(Object.values(model).flatMap((day) => day.creators))) : []),
    [model, showAuthors],
  );

  const canCompose = Boolean(user && (!creatorUsername || creatorUsername === user.username));
  const list = activeDate && (
    <>
      <RosterDayPanel date={activeDate} />
      <NotePanelList
        notes={activeNotes}
        selectionKey={activeDate}
        timeDisplay="time"
        emptyText={canCompose ? undefined : t("calendar.no-notes-on-day")}
        compose={
          canCompose
            ? {
                cacheKey: `calendar-day-editor:${activeDate}`,
                label: t("calendar.new-note-on-day"),
                defaults: { defaultCreateTime },
                onSavingChange: setSaving,
              }
            : undefined
        }
      />
    </>
  );

  return (
    <>
      {/* From xl the section is viewport-tall so the grid can fill it, and pads its end edge by
          the open panel's width so the card floats beside the grid rather than over it. The
          padding reads the panel's own custom property, so it follows a resize drag live. */}
      <section
        className="flex w-full min-w-0 flex-col gap-1 xl:h-[calc(100dvh-3.5rem)]"
        style={{ paddingInlineEnd: xl && panelOpen ? `calc(${NOTE_PANEL_WIDTH_CSS} + ${NOTE_PANEL_INSET}px)` : undefined }}
      >
        <RosterTodayBanner className="shrink-0" />
        <CalendarHeader month={month} monthLabel={monthLabel} today={today} activeDate={activeDate} closable={md} />
        <MentionResolutionProvider contents={NO_CONTENTS} userNames={authorNames}>
          <CalendarGrid
            month={month}
            monthLabel={monthLabel}
            today={today}
            counts={statistics.activityStats}
            model={model}
            pending={isLoading || !isUserSettingsInitialized}
            selectedDate={activeDate}
            showRows={md}
            showAuthors={showAuthors}
          />
        </MentionResolutionProvider>
        {error && <NoteListError error={error} onRetry={refetch} />}
        {activeDate && !md && (
          <section aria-label={dateLabel} className="mt-4 flex w-full flex-col">
            <header className="mb-3 border-b border-border/70 pb-3">
              <h2 className={NOTE_PANEL_TITLE_CLASS}>{dateLabel}</h2>
            </header>
            {list}
          </section>
        )}
      </section>

      {md && (
        <NotePanel open={panelOpen} title={dateLabel} reservedWidth={RESERVED_BESIDE_PANEL} busy={saving} onClose={closeDay}>
          {list}
        </NotePanel>
      )}
    </>
  );
};

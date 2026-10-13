import { ArrowUpIcon, LoaderCircleIcon } from "lucide-react";
import { type ReactElement, type ReactNode, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { MentionResolutionProvider } from "@/components/NoteContent/MentionResolutionContext";
import { Button } from "@/components/ui/button";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useNewNote } from "@/contexts/NewNoteContext";
import { isSearchFilter, useNoteFilterContext } from "@/contexts/NoteFilterContext";
import { useView } from "@/contexts/ViewContext";
import { useDelayedFlag } from "@/hooks/useDelayedFlag";
import { useInfiniteNotes } from "@/hooks/useNoteQueries";
import { hoistNoteToFront } from "@/hooks/useNoteSorting";
import { combineCELFilters } from "@/lib/cel-filter";
import { DEFAULT_LIST_NOTES_PAGE_SIZE, LOADING_INDICATOR_DELAY_MS } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import ColumnGrid, { ColumnGridUntrappedProvider, columnCountForWidth, GRID_GAP } from "../ColumnGrid";
import NoteFilters from "../NoteFilters";
import Placeholder from "../Placeholder";
import NoteListError, { isAccessDeniedError } from "./NoteListError";
import { estimateNoteCardHeight } from "./noteCardHeight";

// Note identity for React keys and grid planning. The pages use it for their renderer keys too,
// so flow-list and grid identity can never drift apart. Deliberately name-only: content updates
// reconcile in place (updateTime is a protobuf Timestamp object, not usable in a template string).
export const getNoteKey = (note: Note) => note.name;

// Columns never stretch past this, so 2 columns on a wide monitor stay readable and the
// grid centers in the leftover space instead of filling it.
const MAX_COLUMN_WIDTH = 420;

const Loader = () => (
  <div className="w-full flex flex-row justify-center items-center py-8">
    <LoaderCircleIcon className="h-6 w-6 animate-spin text-muted-foreground" />
  </div>
);

interface Props {
  renderer: (note: Note, options: { compact: boolean }) => ReactElement;
  listSort?: (list: Note[]) => Note[];
  state?: State;
  orderBy?: string;
  filter?: string;
  contextFilter?: string;
  pageSize?: number;
  showCreator?: boolean;
  enabled?: boolean;
  /** Route-owned content rendered before the list and inside column one in grid mode. */
  renderLeading?: (options: { useGrid: boolean }) => ReactNode;
  /** Route-owned content spanning the full list width above every column (e.g. a page identity block). */
  renderHeader?: (options: { useGrid: boolean }) => ReactNode;
  /** Replaces the generic empty-state message when the route knows why the list is empty. */
  emptyMessage?: string;
}

function useAutoFetchWhenNotScrollable({
  enabled,
  hasNextPage,
  isFetchingNextPage,
  noteCount,
  onFetchNext,
}: {
  enabled: boolean;
  hasNextPage: boolean | undefined;
  isFetchingNextPage: boolean;
  noteCount: number;
  onFetchNext: () => Promise<unknown>;
}) {
  const autoFetchTimeoutRef = useRef<number | null>(null);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  const isPageScrollable = useCallback(() => {
    const documentHeight = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
    return documentHeight > window.innerHeight + 100;
  }, []);

  const checkAndFetchIfNeeded = useCallback(async () => {
    if (autoFetchTimeoutRef.current) {
      clearTimeout(autoFetchTimeoutRef.current);
    }

    await new Promise((resolve) => setTimeout(resolve, 200));

    const shouldFetch = enabledRef.current && !isPageScrollable() && hasNextPage && !isFetchingNextPage && noteCount > 0;

    if (shouldFetch) {
      await onFetchNext();

      if (enabledRef.current) {
        autoFetchTimeoutRef.current = window.setTimeout(() => {
          void checkAndFetchIfNeeded();
        }, 500);
      }
    }
  }, [enabled, hasNextPage, isFetchingNextPage, noteCount, isPageScrollable, onFetchNext]);

  useEffect(() => {
    if (enabled && !isFetchingNextPage && noteCount > 0) {
      void checkAndFetchIfNeeded();
    }
  }, [enabled, noteCount, isFetchingNextPage, checkAndFetchIfNeeded]);

  useEffect(() => {
    if (!enabled && autoFetchTimeoutRef.current) {
      clearTimeout(autoFetchTimeoutRef.current);
      autoFetchTimeoutRef.current = null;
    }
  }, [enabled]);

  useEffect(() => {
    return () => {
      if (autoFetchTimeoutRef.current) {
        clearTimeout(autoFetchTimeoutRef.current);
      }
    };
  }, []);
}

const PagedNoteList = (props: Props) => {
  const t = useTranslate();
  const { isUserSettingsInitialized } = useAuth();
  const { filters, noteView, removeFilter } = useNoteFilterContext();
  const { setQuickFindOpen } = useAppSidebar();
  const { maxColumns, compactMode } = useView();
  // maxColumns is a ceiling: 1 = single reading column, 0 = as many as fit. The single
  // column renders in normal document flow; anything wider becomes the packed grid.
  const multiColumn = maxColumns !== 1;

  // Measure the available width: when it only fits one column anyway, render the flow
  // layout rather than a degenerate one-column grid (capped tiles, composer-as-tile).
  // Only the boolean is stored, so continuous resizes re-render nothing until the
  // one-column threshold is actually crossed.
  const layoutMeasureRef = useRef<HTMLDivElement>(null);
  const [fitsGridWidth, setFitsGridWidth] = useState<boolean | undefined>(undefined);
  useLayoutEffect(() => {
    const el = layoutMeasureRef.current;
    if (!el) return;
    const apply = (nextWidth: number) => setFitsGridWidth(columnCountForWidth(nextWidth) >= 2);
    apply(el.clientWidth);
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver((entries) => apply(entries[0]?.contentRect.width ?? el.clientWidth));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  const useGrid = multiColumn && (fitsGridWidth ?? true);
  // Grid tiles are always bounded/compact; the narrow-width fallback behaves exactly like
  // maxColumns = 1, so it respects the user's own compact setting. Centralized here so the
  // pages don't each repeat the policy.
  const effectiveCompact = compactMode || useGrid;

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, isError, error, isFetchNextPageError, refetch } =
    useInfiniteNotes(
      {
        state: props.state || State.NORMAL,
        orderBy: props.orderBy || "create_time desc",
        filter: combineCELFilters(props.contextFilter, props.filter),
        pageSize: props.pageSize || DEFAULT_LIST_NOTES_PAGE_SIZE,
      },
      { enabled: props.enabled ?? true },
    );

  // Tag settings decide whether sensitive note content must be blurred. Keep that
  // privacy boundary, but do not wait for unrelated note views or instance settings.
  const isDisplayPending = isLoading || !isUserSettingsInitialized;
  // A failed first page leaves nothing to show; a failed later page keeps what already
  // loaded. Either way, automatic pagination pauses until the user retries.
  const isInitialError = isError && !isFetchNextPageError;
  const canPaginate = !isDisplayPending && !isError;
  const showLoader = useDelayedFlag(isDisplayPending, LOADING_INDICATOR_DELAY_MS);

  // Flatten pages into a single array of notes
  const notes = useMemo(() => data?.pages.flatMap((page) => page.notes) || [], [data]);

  // Apply custom sorting if provided, otherwise use notes directly, then hoist
  // a freshly created note to the very top so it stays visible above pins.
  const { newNoteName } = useNewNote();
  const sortedNoteList = useMemo(() => {
    const sorted = props.listSort ? props.listSort(notes) : notes;
    return hoistNoteToFront(sorted, newNoteName);
  }, [notes, props.listSort, newNoteName]);

  // Auto-fetch hook: fetches more content when page isn't scrollable
  useAutoFetchWhenNotScrollable({
    enabled: canPaginate,
    hasNextPage,
    isFetchingNextPage,
    noteCount: sortedNoteList.length,
    onFetchNext: fetchNextPage,
  });

  // Infinite scroll: fetch more when user scrolls near bottom
  useEffect(() => {
    if (!canPaginate || !hasNextPage) return;

    const handleScroll = () => {
      const nearBottom = window.innerHeight + window.scrollY >= document.body.offsetHeight - 300;
      if (nearBottom && !isFetchingNextPage) {
        fetchNextPage();
      }
    };

    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, [canPaginate, hasNextPage, isFetchingNextPage, fetchNextPage]);

  const leadingContent = props.renderLeading?.({ useGrid });
  const headerContent = props.renderHeader?.({ useGrid });

  // A freshly created note is hoisted to the front; pin it to the top of column one so it
  // appears right under the composer instead of dropping into a random (shortest) column.
  const displayNoteList = isDisplayPending || isInitialError ? [] : sortedNoteList;
  const firstNote = displayNoteList[0];
  const priorityKey = newNoteName && firstNote?.name === newNoteName ? getNoteKey(firstNote) : undefined;

  // Stable reference so MentionResolutionProvider's note (keyed on the array) actually holds.
  const contents = useMemo(() => displayNoteList.map((note) => note.content), [displayNoteList]);
  const userNames = useMemo(
    () =>
      Array.from(
        new Set(
          displayNoteList.flatMap((note) => [
            ...(props.showCreator ? [note.creator] : []),
            ...(note.reactions ?? []).map((reaction) => reaction.creator),
          ]),
        ),
      ),
    [props.showCreator, displayNoteList],
  );

  const hasFilters = filters.length > 0 || noteView !== undefined;
  // A list the visitor may not read shows the same empty state as a list with no notes.
  const accessDenied = isError && isAccessDeniedError(error);
  const emptyPlaceholder =
    !isDisplayPending && (!isError || accessDenied) && !isFetchingNextPage && !hasNextPage && displayNoteList.length === 0 ? (
      <Placeholder scene={hasFilters ? "search" : "note"} message={props.emptyMessage ?? t("message.no-data")} className="w-full" />
    ) : null;
  const initialLoader = isDisplayPending && showLoader ? <Loader /> : null;
  // Only a query the user typed can be edited or cleared from the error; facet and scope
  // filters are fixed by the route.
  const hasSearch = filters.some(isSearchFilter);
  const errorNotice =
    !isDisplayPending && isError && !accessDenied ? (
      <NoteListError
        error={error}
        onRetry={isFetchNextPageError ? fetchNextPage : refetch}
        onEditQuery={hasSearch ? () => setQuickFindOpen(true) : undefined}
        onClearQuery={hasSearch ? () => removeFilter(isSearchFilter) : undefined}
      />
    ) : null;
  const initialError = isInitialError ? errorNotice : null;
  const pageError = isFetchNextPageError ? errorNotice : null;

  // Column one is the action column: the composer and any active filters head it, and the
  // empty state follows them. The newest note also lands directly beneath them (priorityKey
  // above). Every vertical seam inside the stack uses GRID_GAP so y-spacing matches the
  // grid's x-spacing exactly.
  const gridLeading =
    leadingContent || hasFilters || initialLoader || emptyPlaceholder || initialError ? (
      <div className="flex w-full flex-col" style={{ gap: GRID_GAP }}>
        {leadingContent}
        <NoteFilters />
        {initialLoader}
        {initialError}
        {emptyPlaceholder}
      </div>
    ) : undefined;

  // Pagination controls are identical across both layouts.
  const footer = (
    <>
      {pageError}
      {isFetchingNextPage && <Loader />}
      {!isFetchingNextPage && (hasNextPage || displayNoteList.length > 0) && <BackToTop />}
    </>
  );

  const children = (
    <MentionResolutionProvider contents={contents} userNames={userNames}>
      <div ref={layoutMeasureRef} className="w-full">
        <div className={cn("flex flex-col justify-start w-full mx-auto", useGrid ? "max-w-none" : "max-w-2xl")}>
          {useGrid ? (
            <>
              <ColumnGridUntrappedProvider>
                <ColumnGrid
                  items={displayNoteList}
                  getKey={getNoteKey}
                  renderItem={(note) => props.renderer(note, { compact: effectiveCompact })}
                  estimateHeight={estimateNoteCardHeight}
                  header={headerContent}
                  leading={gridLeading}
                  priorityKey={priorityKey}
                  maxColumns={maxColumns}
                  maxColumnWidth={MAX_COLUMN_WIDTH}
                />
              </ColumnGridUntrappedProvider>
              {!isDisplayPending && footer}
            </>
          ) : (
            <>
              {headerContent}
              {leadingContent}
              <NoteFilters className="mb-2" />
              {initialLoader}
              {initialError}
              {displayNoteList.map((note) => props.renderer(note, { compact: effectiveCompact }))}
              {emptyPlaceholder}
              {!isDisplayPending && footer}
            </>
          )}
        </div>
      </div>
    </MentionResolutionProvider>
  );

  return children;
};

const BackToTop = () => {
  const t = useTranslate();
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    const handleScroll = () => {
      const shouldShow = window.scrollY > 400;
      setIsVisible(shouldShow);
    };

    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  const scrollToTop = () => {
    window.scrollTo({
      top: 0,
      behavior: "smooth",
    });
  };

  // Render nothing at all while hidden, so the list's end carries no phantom spacing.
  if (!isVisible) {
    return null;
  }

  return (
    <div className="my-4 flex w-full flex-row items-center justify-center opacity-70">
      <Button variant="ghost" onClick={scrollToTop}>
        {t("router.back-to-top")}
        <ArrowUpIcon className="ml-1 w-4 h-auto" />
      </Button>
    </div>
  );
};

export default PagedNoteList;

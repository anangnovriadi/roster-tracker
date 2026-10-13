import type { Memo } from "@/types/proto/api/memo_service_pb";

/** How the header names the memo's time: relative to now, or just the clock time for lists that already name the day. */
export type MemoTimeDisplay = "relative" | "time";

export interface MemoViewProps {
  memo: Memo;
  compact?: boolean;
  timeDisplay?: MemoTimeDisplay;
  showCreator?: boolean;
  showVisibility?: boolean;
  showPinned?: boolean;
  className?: string;
  parentPage?: string;
}

export interface MemoViewHandle {
  openEditor: () => void;
}

export interface MemoHeaderProps {
  timeDisplay?: MemoTimeDisplay;
  showCreator?: boolean;
  showVisibility?: boolean;
}

export interface MemoBodyProps {
  compact?: boolean;
}

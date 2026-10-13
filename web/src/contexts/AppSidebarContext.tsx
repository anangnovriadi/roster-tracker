import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import type { NoteParentStatus } from "@/components/NoteParentPlaceholder";
import { useLocalStorage } from "@/hooks/useLocalStorage";
import type { Note } from "@/types/proto/api/note_service_pb";

export type AttachmentSection = "all" | "media" | "documents" | "unused";
export type InboxFilter = "all" | "unread" | "archived";

export interface NoteDetailSidebarDescriptor {
  note: Note;
  parentNote?: Note;
  parentStatus?: NoteParentStatus;
  onParentRetry?: () => void;
  from?: string;
  commentCount?: number;
  readonly?: boolean;
  onEdit?: () => void;
  onCommentsOpen?: () => void;
  onCommentCreate?: () => void;
}

interface AppSidebarContextValue {
  attachmentSection: AttachmentSection;
  setAttachmentSection: (section: AttachmentSection) => void;
  inboxFilter: InboxFilter;
  setInboxFilter: (filter: InboxFilter) => void;
  noteDetail: NoteDetailSidebarDescriptor | undefined;
  setNoteDetail: (descriptor?: NoteDetailSidebarDescriptor) => void;
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  closeMobileThen: (action: () => void) => void;
  completeMobileClose: (open: boolean) => void;
  quickFindOpen: boolean;
  setQuickFindOpen: (open: boolean) => void;
  /** The desktop sidebar is hidden; remembered across visits. */
  desktopCollapsed: boolean;
  setDesktopCollapsed: (collapsed: boolean) => void;
}

const AppSidebarContext = createContext<AppSidebarContextValue | null>(null);

export const AppSidebarProvider = ({ children }: { children: ReactNode }) => {
  const location = useLocation();
  const [attachmentSection, setAttachmentSection] = useState<AttachmentSection>("all");
  const [inboxFilter, setInboxFilter] = useState<InboxFilter>("all");
  const [noteDetail, setNoteDetailState] = useState<NoteDetailSidebarDescriptor>();
  const [mobileOpen, setMobileOpen] = useState(false);
  const pendingMobileCloseActionRef = useRef<(() => void) | undefined>(undefined);
  const scheduledMobileCloseActionFrameRef = useRef<number | undefined>(undefined);
  const [quickFindOpen, setQuickFindOpen] = useState(false);
  const [desktopCollapsed, setDesktopCollapsed] = useLocalStorage("sidebar-collapsed", false);

  useEffect(() => {
    pendingMobileCloseActionRef.current = undefined;
    if (scheduledMobileCloseActionFrameRef.current !== undefined) {
      window.cancelAnimationFrame(scheduledMobileCloseActionFrameRef.current);
      scheduledMobileCloseActionFrameRef.current = undefined;
    }
    setMobileOpen(false);
    setAttachmentSection("all");
    setInboxFilter("all");

    return () => {
      pendingMobileCloseActionRef.current = undefined;
      if (scheduledMobileCloseActionFrameRef.current !== undefined) {
        window.cancelAnimationFrame(scheduledMobileCloseActionFrameRef.current);
        scheduledMobileCloseActionFrameRef.current = undefined;
      }
    };
  }, [location.pathname]);

  const setNoteDetail = useCallback((descriptor?: NoteDetailSidebarDescriptor) => {
    setNoteDetailState(descriptor);
  }, []);

  const closeMobileThen = useCallback(
    (action: () => void) => {
      if (!mobileOpen) {
        action();
        return;
      }

      pendingMobileCloseActionRef.current = action;
      setMobileOpen(false);
    },
    [mobileOpen],
  );

  const completeMobileClose = useCallback((open: boolean) => {
    if (open) return;
    const action = pendingMobileCloseActionRef.current;
    pendingMobileCloseActionRef.current = undefined;
    if (!action) return;

    // Base UI restores focus in a microtask after the Sheet unmounts. Run the destination
    // action on the next frame so an editor's focus cannot be overwritten by that cleanup.
    scheduledMobileCloseActionFrameRef.current = window.requestAnimationFrame(() => {
      scheduledMobileCloseActionFrameRef.current = undefined;
      action();
    });
  }, []);

  const value = useMemo(
    () => ({
      attachmentSection,
      setAttachmentSection,
      inboxFilter,
      setInboxFilter,
      noteDetail,
      setNoteDetail,
      mobileOpen,
      setMobileOpen,
      closeMobileThen,
      completeMobileClose,
      quickFindOpen,
      setQuickFindOpen,
      desktopCollapsed,
      setDesktopCollapsed,
    }),
    [
      attachmentSection,
      inboxFilter,
      noteDetail,
      setNoteDetail,
      mobileOpen,
      closeMobileThen,
      completeMobileClose,
      quickFindOpen,
      desktopCollapsed,
      setDesktopCollapsed,
    ],
  );

  return <AppSidebarContext.Provider value={value}>{children}</AppSidebarContext.Provider>;
};

export const useAppSidebar = () => {
  const context = useContext(AppSidebarContext);
  if (!context) {
    throw new Error("useAppSidebar must be used within AppSidebarProvider");
  }
  return context;
};

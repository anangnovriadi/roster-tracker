import type { NoteSuggestion } from "@/lib/note-suggestions";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import type { Location, Note, Visibility } from "@/types/proto/api/note_service_pb";
import type { EditorFileOrigin } from "../Editor/extensions";
import type { LocalFile } from "./attachment";

export interface NoteEditorProps {
  className?: string;
  cacheKey?: string;
  placeholder?: string;
  /** Existing note to edit. When provided, the editor initializes from it without fetching. */
  note?: Note;
  parentNoteName?: string;
  /** Context-derived suggestions for a new top-level note. */
  suggestions?: readonly NoteSuggestion[];
  /** Assigns a newly created top-level note to this Space. Ignored for edits and comments. */
  defaultSpace?: string;
  /** Seeds a new note once; restored draft metadata takes precedence. */
  defaultLocation?: Location;
  /** A callback can decide whether focus is still appropriate after draft restoration. */
  autoFocus?: boolean | (() => boolean);
  /**
   * Marks the instance as *hosted*: a host (the global composer dialog) presents
   * the editor in the focus-mode layout and owns that frame. The editor mounts
   * straight into focus mode, drops the view toggles that only make sense inline,
   * and exits by calling this to dismiss the host rather than collapsing in place.
   */
  onFocusModeExit?: () => void;
  /**
   * Reports inline focus-mode changes. A host reacts to the editor taking over
   * the viewport, e.g. the note grid untraps its tile so the fixed focus-mode
   * surface is not contained by a transformed ancestor.
   */
  onFocusModeChange?: (isFocusMode: boolean) => void;
  /**
   * Default `createTime` for a *new* note (create mode only). When set, the
   * editor seeds both `createTime` and `updateTime` to this value and renders
   * the timestamp popover so the user can adjust before saving. Tracked live:
   * if the prop changes after mount, the editor's timestamps re-sync. Ignored
   * in edit mode (when `note` is set).
   */
  defaultCreateTime?: Date;
  onConfirm?: (noteName: string) => void;
  onCancel?: () => void;
  /** Reports save activity so external presentations can prevent dismissal mid-transaction. */
  onSavingChange?: (isSaving: boolean) => void;
}

export interface EditorContentProps {
  placeholder?: string;
  /** Invoked by the in-editor save shortcut (Cmd/Ctrl+Enter). */
  onSubmit: () => void;
  onFiles: (files: File[], origin: EditorFileOrigin) => void;
}

/**
 * The ＋ menu's view toggles. They change how the editor presents itself
 * inline, so a hosted editor omits the whole group and both items disappear
 * together — there is no way to offer one without the other.
 */
export interface EditorViewToggles {
  onToggleFocusMode: () => void;
  /** Whether the formatting toolbar is shown in normal mode (persisted preference). */
  isFormattingToolbarVisible: boolean;
  onToggleFormattingToolbar: () => void;
}

export interface EditorToolbarProps {
  onSave: () => void;
  onCancel?: () => void;
  noteName?: string;
  /** Set when the editor composes a comment on this note; picks the commit verb. */
  parentNoteName?: string;
  viewToggles?: EditorViewToggles;
  onInsertImages: (files: File[]) => void;
}

export interface EditorMetadataProps {
  onTranscribeAttachment?: (attachment: Attachment) => void;
  transcribingAttachment?: string;
  transcriptionDisabled?: boolean;
  noteName?: string;
  uploadingLocalFileURLs: ReadonlySet<string>;
  onInsertAttachments: (attachments: Attachment[]) => void;
  onInsertLocalFiles: (localFiles: LocalFile[]) => void;
}

export interface FocusModeOverlayProps {
  isActive: boolean;
  onToggle: () => void;
}

export interface FocusModeExitButtonProps {
  isActive: boolean;
  onToggle: () => void;
  title: string;
}

export interface InsertMenuProps {
  isUploading?: boolean;
  isSaving?: boolean;
  viewToggles?: EditorViewToggles;
  onInsertImages: (files: File[]) => void;
}

export interface AudienceMenuProps {
  value: Visibility;
  onChange: (visibility: Visibility) => void;
  disabled?: boolean;
}

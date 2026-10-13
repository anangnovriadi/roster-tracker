export const NOTE_COMMENTS_ANCHOR_ID = "note-comments";
export const LEGACY_NOTE_COMMENTS_ANCHOR_ID = "comments";

/** Keep note headings from claiming the current or legacy comment-section fragments. */
export const RESERVED_NOTE_COMMENTS_ANCHOR_IDS = [NOTE_COMMENTS_ANCHOR_ID, LEGACY_NOTE_COMMENTS_ANCHOR_ID] as const;

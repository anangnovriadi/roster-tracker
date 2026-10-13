import { memo, useRef } from "react";
import { useContentSearchTerms } from "@/contexts/NoteFilterContext";
import { cn } from "@/lib/utils";
import { useResolvedMentionUsernames } from "./MentionResolutionContext";
import { NoteMarkdownRenderer } from "./NoteMarkdownRenderer";
import type { NoteContentProps } from "./types";
import { useSearchMatchHighlight } from "./useSearchMatchHighlight";

// Stateless markdown renderer. Truncation is not this component's concern — compact cards
// are bounded by ClampedSection around the whole note body; `compact` here only informs
// the renderer (e.g. footnote links navigate to the detail page instead of scrolling,
// since a collapsed card may hide the target).
const NoteContent = (props: NoteContentProps) => {
  const { className, contentClassName, content, attachments, onClick, onDoubleClick } = props;
  const resolvedMentionUsernames = useResolvedMentionUsernames(content);
  // Whatever the page's search matched is painted here, without altering the rendered tree.
  const contentRef = useRef<HTMLDivElement>(null);
  useSearchMatchHighlight(contentRef, useContentSearchTerms());

  return (
    <div className={`w-full flex flex-col justify-start items-start text-foreground ${className || ""}`}>
      <div
        ref={contentRef}
        data-note-content
        data-note-name={props.noteName}
        className={cn(
          "relative w-full max-w-full wrap-break-word text-base leading-6",
          "[&>*:first-child]:mt-0 [&>*:last-child]:mb-0",
          "[&_.katex-display]:max-w-full",
          "[&_.katex-display]:overflow-x-auto",
          "[&_.katex-display]:overflow-y-hidden",
          // Footnotes: quiet GitHub-style footer — thin separator, smaller muted text, unobtrusive links.
          "[&_.footnotes]:mt-4 [&_.footnotes]:border-t [&_.footnotes]:border-border [&_.footnotes]:pt-2",
          "[&_.footnotes]:text-sm [&_.footnotes]:text-muted-foreground",
          // GitHub renders footnote ref/backref links without an underline (underline on hover only).
          "[&_[data-footnote-ref]]:no-underline [&_[data-footnote-ref]:hover]:underline",
          "[&_.data-footnote-backref]:no-underline [&_.data-footnote-backref:hover]:underline",
          contentClassName,
        )}
        onMouseUp={onClick}
        onDoubleClick={onDoubleClick}
      >
        <NoteMarkdownRenderer
          content={content}
          attachments={attachments}
          resolvedMentionUsernames={resolvedMentionUsernames}
          noteName={props.noteName}
          parentPage={props.parentPage}
          compact={Boolean(props.compact)}
        />
      </div>
    </div>
  );
};

export default memo(NoteContent);

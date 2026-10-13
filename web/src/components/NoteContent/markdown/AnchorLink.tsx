import { Link } from "react-router-dom";
import { markdownStyles } from "@/lib/markdownStyles";
import { cn } from "@/lib/utils";
import { buildNotePath } from "@/router/routes";
import { findAnchorTarget } from "@/utils/markdown-manipulation";
import { createNoteNavigationState } from "../../NoteView/navigation";
import type { ReactMarkdownProps } from "./types";

interface AnchorLinkProps extends React.AnchorHTMLAttributes<HTMLAnchorElement>, ReactMarkdownProps {
  href: string;
  /** Resource name of the enclosing note (e.g. `notes/abc123`), when known. */
  noteName?: string;
  /** Collection page that rendered the enclosing note. */
  parentPage?: string;
  /** Whether the note is rendered as a collapsed feed card. */
  compact?: boolean;
  children: React.ReactNode;
}

/**
 * Renders in-page anchors (footnote references/backrefs, heading links — any `href="#…"`).
 *
 * When the target lives in a fully-rendered note (detail, share, preview, expanded feed card),
 * we scroll to it within that note's own container — scoped so duplicate footnote ids across a
 * feed can't send us to the wrong note. When the note is a collapsed feed card the footnote is
 * below the fold, so we fall back to navigating to the note detail page (with the hash), where
 * NoteDetail scrolls the target into view.
 */
export const AnchorLink = ({ href, noteName, parentPage, compact, children, className, node: _node, ...props }: AnchorLinkProps) => {
  const handleClick = (event: React.MouseEvent<HTMLAnchorElement>) => {
    if (compact) return; // Let the link navigate to the detail page.
    const id = decodeURIComponent(href.slice(1));
    if (!id) return;
    // Scope the lookup to this note's own container so duplicate footnote ids elsewhere in a feed
    // can't steal the scroll.
    const root = event.currentTarget.closest("[data-note-content]");
    const target = root && findAnchorTarget(root, id);
    if (target) {
      event.preventDefault();
      target.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  };

  const classes = cn(markdownStyles.link, className);

  if (noteName) {
    return (
      <Link
        to={`${buildNotePath(noteName)}${href}`}
        state={parentPage ? createNoteNavigationState(parentPage) : undefined}
        onClick={handleClick}
        className={classes}
        {...props}
      >
        {children}
      </Link>
    );
  }

  return (
    <a href={href} onClick={handleClick} className={classes} {...props}>
      {children}
    </a>
  );
};

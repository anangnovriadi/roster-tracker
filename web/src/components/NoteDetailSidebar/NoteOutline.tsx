import { useEffect, useMemo, useRef, useState } from "react";
import type { HeadingItem } from "@/components/NoteContent/pipeline";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { findAnchorTarget, findNoteContentRoot } from "@/utils/markdown-manipulation";

interface NoteOutlineProps {
  headings: HeadingItem[];
  noteName: string;
}

/** Distance from the viewport top of the "reading line" used to decide the active section. */
const READING_LINE_OFFSET = 100;

/**
 * Each heading is a quiet row: the kit's `quiet` treatment at the sidebar's 28px row height,
 * so the section you are reading takes the accent fill through `aria-current`.
 */
const OUTLINE_ROW_CLASSES = cn(buttonVariants({ variant: "quiet", size: "sm" }), "relative w-full justify-start");

/** Outline navigation for note headings (h1–h4) with active-section tracking. */
const NoteOutline = ({ headings, noteName }: NoteOutlineProps) => {
  const [activeSlug, setActiveSlug] = useState<string | null>(null);
  const rafRef = useRef(0);

  const minLevel = useMemo(() => Math.min(...headings.map((heading) => heading.level)), [headings]);

  useEffect(() => {
    const update = () => {
      rafRef.current = 0;
      let current: string | null = null;
      const noteContent = findNoteContentRoot(document, noteName);
      if (!noteContent) return;
      for (const heading of headings) {
        const el = findAnchorTarget(noteContent, heading.slug);
        if (!el) continue;
        if (el.getBoundingClientRect().top > READING_LINE_OFFSET) break;
        current = heading.slug;
      }
      setActiveSlug(current ?? headings[0]?.slug ?? null);
    };
    const requestUpdate = () => {
      if (!rafRef.current) {
        rafRef.current = requestAnimationFrame(update);
      }
    };
    update();
    // Capture-phase listener so scrolls of any nested container are observed too.
    window.addEventListener("scroll", requestUpdate, true);
    window.addEventListener("resize", requestUpdate);
    return () => {
      window.removeEventListener("scroll", requestUpdate, true);
      window.removeEventListener("resize", requestUpdate);
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
    };
  }, [headings, noteName]);

  const handleClick = (e: React.MouseEvent<HTMLAnchorElement>, slug: string) => {
    e.preventDefault();
    const noteContent = findNoteContentRoot(document, noteName);
    const el = noteContent && findAnchorTarget(noteContent, slug);
    if (el) {
      setActiveSlug(slug);
      el.scrollIntoView({ behavior: "smooth", block: "start" });
      window.history.replaceState(null, "", `#${slug}`);
    }
  };

  return (
    <nav className="relative flex flex-col gap-0.5">
      {headings.map((heading, index) => {
        const active = heading.slug === activeSlug;
        return (
          <a
            key={`${heading.slug}-${index}`}
            href={`#${heading.slug}`}
            onClick={(e) => handleClick(e, heading.slug)}
            aria-current={active ? "location" : undefined}
            className={cn(OUTLINE_ROW_CLASSES, heading.level === minLevel && "font-medium")}
            style={{ paddingInlineStart: 8 + (heading.level - minLevel) * 12 }}
          >
            <span
              className={cn(
                "absolute start-0.5 top-1/2 h-[13px] w-[2px] -translate-y-1/2 rounded-full transition-colors",
                active ? "bg-primary" : "bg-border",
              )}
            />
            <span className="min-w-0 flex-1 truncate">{heading.text}</span>
          </a>
        );
      })}
    </nav>
  );
};

export default NoteOutline;

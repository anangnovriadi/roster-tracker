import { EyeIcon } from "lucide-react";
import { useMemo } from "react";
import ClampedSection from "@/components/ClampedSection";
import { AttachmentGallery, NoteMetadataRows } from "@/components/NoteMetadata";
import { separateAttachments } from "@/components/NoteMetadata/Attachment/attachmentHelpers";
import { isReferenceRelation } from "@/components/NoteMetadata/Relation/relationHelpers";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import { filterInlineManagedAttachments } from "@/utils/managed-attachment";
import NoteContent from "../../NoteContent";
import { NoteReactionListView } from "../../NoteReactionListView";
import { useNoteHandlers } from "../hooks";
import { useNoteViewContext } from "../NoteViewContext";
import type { NoteBodyProps } from "../types";

const BlurOverlay: React.FC<{ onClick?: () => void }> = ({ onClick }) => {
  const t = useTranslate();
  return (
    <div className="absolute inset-0 z-10 flex items-center justify-center">
      <Button type="button" variant="outline" size="sm" onClick={onClick}>
        <EyeIcon className="size-3.5" strokeWidth={1.8} />
        {t("note.click-to-show-sensitive-content")}
      </Button>
    </div>
  );
};

const NoteBody: React.FC<NoteBodyProps> = ({ compact }) => {
  const { note, parentPage, showBlurredContent, blurred, readonly, openEditor, openPreview, toggleBlurVisibility } = useNoteViewContext();

  const { handleNoteContentClick, handleNoteContentDoubleClick } = useNoteHandlers({ readonly, openEditor, openPreview });

  const referencedNotes = note.relations.filter(isReferenceRelation);
  // Memoized so AttachmentListView's own useMemo chain keeps its cache across body renders.
  const attachmentOnlyItems = useMemo(
    () => filterInlineManagedAttachments(note.content, note.attachments),
    [note.content, note.attachments],
  );
  const { visual, audio, docs } = useMemo(() => separateAttachments(attachmentOnlyItems), [attachmentOnlyItems]);

  return (
    <div className="w-full flex flex-col justify-start items-start gap-2">
      <div data-slot="note-body" className="relative w-full">
        <div
          className={cn(
            "w-full flex flex-col justify-start items-start gap-2",
            blurred && !showBlurredContent && "blur-lg transition-all duration-200",
          )}
        >
          {/* Compact bounds the whole body — attachments included — behind one Show more.
              Reactions stay outside so they never hide under the fade. */}
          <ClampedSection enabled={Boolean(compact)}>
            <NoteContent
              noteName={note.name}
              parentPage={parentPage}
              content={note.content}
              attachments={note.attachments}
              onClick={handleNoteContentClick}
              onDoubleClick={handleNoteContentDoubleClick}
              compact={Boolean(compact)}
            />
            <AttachmentGallery visual={visual} onImagePreview={openPreview} />
            <NoteMetadataRows
              audio={audio}
              docs={docs}
              relations={referencedNotes}
              currentNoteName={note.name}
              parentPage={parentPage}
              location={note.location}
            />
          </ClampedSection>
        </div>

        {blurred && !showBlurredContent && <BlurOverlay onClick={toggleBlurVisibility} />}
      </div>

      <NoteReactionListView note={note} reactions={note.reactions} />
    </div>
  );
};

export default NoteBody;

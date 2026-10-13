import { FileIcon } from "lucide-react";
import type { AttachmentLibraryListItem } from "@/hooks/useAttachmentLibrary";
import type { User } from "@/types/proto/api/user_service_pb";
import { AttachmentCreator, AttachmentMetadataLine, AttachmentOpenButton, AttachmentSourceChip } from "./AttachmentLibraryPrimitives";

type AttachmentCreators = Map<string, User | undefined>;

export const AttachmentDocumentRows = ({ items, creators }: { items: AttachmentLibraryListItem[]; creators?: AttachmentCreators }) => {
  return (
    <div className="space-y-3">
      {items.map((item) => (
        <article
          key={item.attachment.name}
          className="flex items-center gap-2.5 rounded-[18px] border border-border/60 bg-background/90 p-3 shadow-sm shadow-black/[0.02]"
        >
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted/45 text-muted-foreground">
            <FileIcon className="h-4.5 w-4.5" />
          </div>

          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium text-foreground" title={item.attachment.filename}>
              {item.attachment.filename}
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
              <AttachmentMetadataLine className="min-w-0 max-w-full" items={[item.fileTypeLabel, item.fileSizeLabel, item.createdLabel]} />
              <AttachmentCreator creatorName={item.attachment.creator} user={creators?.get(item.attachment.creator)} />
              <AttachmentSourceChip noteName={item.noteName} />
            </div>
          </div>

          <AttachmentOpenButton href={item.sourceUrl} />
        </article>
      ))}
    </div>
  );
};

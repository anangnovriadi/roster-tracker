import { useMemo } from "react";
import { useNearViewport } from "@/hooks/useNearViewport";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import type { Location, NoteRelation } from "@/types/proto/api/note_service_pb";
import { AttachmentRows } from "./Attachment/AttachmentListView";
import LocationDisplayView from "./Location/LocationDisplayView";
import { METADATA_ROW_LIST_CLASSES } from "./MetadataSection";
import RelationRows, { getRelationRowItems } from "./Relation/RelationRows";

interface NoteMetadataRowsProps {
  audio: Attachment[];
  docs: Attachment[];
  relations: NoteRelation[];
  currentNoteName: string;
  parentPage?: string;
  location?: Location;
}

/**
 * Everything attached to a note that is not its content or media, as one list of 28px
 * rows 2px apart: audio, files, related notes, and the location. Renders nothing when
 * the note has none of them, so the body never carries an empty list.
 */
const NoteMetadataRows = ({ audio, docs, relations, currentNoteName, parentPage, location }: NoteMetadataRowsProps) => {
  const { ref: viewportRef, isNearViewport } = useNearViewport<HTMLDivElement>();
  const relationItems = useMemo(() => getRelationRowItems(relations, currentNoteName), [relations, currentNoteName]);

  if (audio.length === 0 && docs.length === 0 && relationItems.length === 0 && !location) {
    return null;
  }

  return (
    <div ref={viewportRef} className={METADATA_ROW_LIST_CLASSES}>
      <AttachmentRows audio={audio} docs={docs} />
      {relationItems.length > 0 && <RelationRows items={relationItems} parentPage={parentPage} enabled={isNearViewport} />}
      {location && <LocationDisplayView location={location} />}
    </div>
  );
};

export default NoteMetadataRows;

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { createContext, useContext } from "react";
import { useLocation } from "react-router-dom";
import { useView } from "@/contexts/ViewContext";
import type { Note } from "@/types/proto/api/note_service_pb";
import { NoteRelation_Type } from "@/types/proto/api/note_service_pb";
import type { User } from "@/types/proto/api/user_service_pb";
import type { PreviewMediaItem } from "@/utils/media-item";
import { RELATIVE_TIME_THRESHOLD_MS } from "./constants";
import { isNoteDetailPath } from "./navigation";

export interface NoteViewContextValue {
  note: Note;
  creator: User | undefined;
  currentUser: User | undefined;
  parentPage: string;
  isArchived: boolean;
  readonly: boolean;
  showBlurredContent: boolean;
  blurred: boolean;
  openEditor: () => void;
  toggleBlurVisibility: () => void;
  openPreview: (items: string | string[] | PreviewMediaItem[], index?: number) => void;
}

export const NoteViewContext = createContext<NoteViewContextValue | null>(null);

export const useNoteViewContext = (): NoteViewContextValue => {
  const context = useContext(NoteViewContext);
  if (!context) {
    throw new Error("useNoteViewContext must be used within NoteViewContext.Provider");
  }
  return context;
};

export const computeCommentAmount = (note: Note): number =>
  note.relations.filter((r) => r.type === NoteRelation_Type.COMMENT && r.relatedNote?.name === note.name).length;

export const useNoteViewDerived = () => {
  const { note, isArchived, readonly } = useNoteViewContext();
  const { timeBasis } = useView();
  const location = useLocation();

  const isInNoteDetailPage = isNoteDetailPath(location.pathname, note.name);
  const commentAmount = computeCommentAmount(note);

  const createTime = note.createTime ? timestampDate(note.createTime) : undefined;
  const updateTime = note.updateTime ? timestampDate(note.updateTime) : undefined;
  const displayTime = timeBasis === "update_time" ? updateTime : createTime;
  const isDisplayingUpdatedTime =
    timeBasis === "update_time" && !!createTime && !!updateTime && updateTime.getTime() !== createTime.getTime();
  const relativeTimeFormat: "datetime" | "auto" =
    displayTime && Date.now() - displayTime.getTime() > RELATIVE_TIME_THRESHOLD_MS ? "datetime" : "auto";

  return {
    isArchived,
    readonly,
    isInNoteDetailPage,
    commentAmount,
    createTime,
    updateTime,
    displayTime,
    isDisplayingUpdatedTime,
    relativeTimeFormat,
  };
};

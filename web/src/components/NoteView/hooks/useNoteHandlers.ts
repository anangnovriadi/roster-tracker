import { useCallback } from "react";
import { useInstance } from "@/contexts/InstanceContext";
import type { PreviewMediaItem } from "@/utils/media-item";

interface UseNoteHandlersOptions {
  readonly: boolean;
  openEditor: () => void;
  openPreview: (items: string | string[] | PreviewMediaItem[], index?: number) => void;
}

export const useNoteHandlers = (options: UseNoteHandlersOptions) => {
  const { readonly, openEditor, openPreview } = options;
  const { noteRelatedSetting } = useInstance();

  const handleNoteContentClick = useCallback(
    (e: React.MouseEvent) => {
      const targetEl = e.target as HTMLElement;
      if (targetEl.tagName === "IMG") {
        const linkElement = targetEl.closest("a");
        if (linkElement) return; // If image is inside a link, don't show preview
        const imgUrl = targetEl.getAttribute("src");
        if (imgUrl) openPreview(imgUrl);
      }
    },
    [openPreview],
  );

  const handleNoteContentDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      if (readonly) return;
      if (noteRelatedSetting.enableDoubleClickEdit) {
        e.preventDefault();
        openEditor();
      }
    },
    [readonly, openEditor, noteRelatedSetting.enableDoubleClickEdit],
  );

  return { handleNoteContentClick, handleNoteContentDoubleClick };
};

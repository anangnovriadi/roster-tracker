import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { toast } from "react-hot-toast";
import { useNewNote } from "@/contexts/NewNoteContext";
import { attachmentKeys } from "@/hooks/useAttachmentQueries";
import { noteKeys } from "@/hooks/useNoteQueries";
import { userKeys } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import type { Visibility } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import { errorService, noteService, validationService } from "../services";
import { useEditorContext } from "../state";
import { restampDefaultCreateTime } from "../utils/deriveDefaultCreateTime";

/** How long a closing host shows "Saved" before it unmounts the editor. */
const SAVED_CONFIRMATION_MS = 900;

interface UseNoteSaveOptions {
  noteName?: string;
  parentNoteName?: string;
  defaultSpace?: string;
  defaultVisibility?: Visibility;
  defaultCreateTime?: Date;
  discardDraft: () => void;
  onConfirm?: (noteName: string) => void;
  onCancel?: () => void;
}

/**
 * Owns the editor's save transaction and its post-save cache/state updates.
 * Keeping this workflow outside the shell makes saving identical whether it is
 * triggered by the toolbar or the editor keyboard shortcut.
 */
export function useNoteSave({
  noteName,
  parentNoteName,
  defaultSpace,
  defaultVisibility,
  defaultCreateTime,
  discardDraft,
  onConfirm,
  onCancel,
}: UseNoteSaveOptions): () => Promise<void> {
  const t = useTranslate();
  const queryClient = useQueryClient();
  const { markNewNote } = useNewNote();
  const { actions, dispatch, getState } = useEditorContext();

  return useCallback(async () => {
    const state = getState();
    // A repeated shortcut during the saved confirmation is not an error worth
    // a toast; the save already landed and the host is closing.
    if (state.ui.justSaved) return;
    const { valid, reason, detail } = validationService.canSave(state);
    if (!valid) {
      toast.error(reason ? t(reason, detail ? { url: detail } : undefined) : t("editor.validation.cannot-save"));
      return;
    }

    dispatch(actions.setLoading("saving", true));

    // Files that finished uploading, keyed by preview URL. If the save fails
    // after they uploaded, they move onto the draft as attachments so that a
    // retry does not upload them again (a refused create would otherwise leak
    // one copy per attempt into the user's storage).
    const uploaded = new Map<string, Attachment>();
    const keepUploadedFiles = () => {
      if (uploaded.size === 0) return;
      const current = getState();
      const moved = current.localFiles.filter((file) => uploaded.has(file.previewUrl));
      if (moved.length === 0) return;
      dispatch(
        actions.setMetadata({
          attachments: [...current.metadata.attachments, ...moved.map((file) => uploaded.get(file.previewUrl) as Attachment)],
        }),
      );
      dispatch(actions.setLocalFiles(current.localFiles.filter((file) => !uploaded.has(file.previewUrl))));
    };

    try {
      // A new note composed on a selected day keeps the day but is stamped with
      // the time of saving, so consecutive notes do not share one createTime.
      const now = new Date();
      const timestamps = noteName
        ? state.timestamps
        : {
            createTime: restampDefaultCreateTime(state.timestamps.createTime, defaultCreateTime, now),
            updateTime: restampDefaultCreateTime(state.timestamps.updateTime, defaultCreateTime, now),
          };
      const result = await noteService.save(
        { ...state, timestamps },
        {
          noteName,
          parentNoteName,
          space: defaultSpace,
          onFileUploaded: (file, attachment) => uploaded.set(file.previewUrl, attachment),
        },
      );

      if (!result.hasChanges) {
        toast.error(t("editor.no-changes-detected"));
        onCancel?.();
        return;
      }

      // Prevent the autosave unmount flush from restoring the saved draft.
      discardDraft();
      // A moved note may leave the feed it was edited in; say where it went.
      if (result.moved) {
        toast.success(t("note.moved"));
      }

      const invalidationPromises: Promise<unknown>[] = [
        queryClient.invalidateQueries({ queryKey: noteKeys.lists() }),
        queryClient.invalidateQueries({ queryKey: userKeys.stats() }),
        queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() }),
      ];
      if (noteName) {
        invalidationPromises.push(queryClient.invalidateQueries({ queryKey: noteKeys.detail(noteName) }));
      }
      if (parentNoteName) {
        invalidationPromises.push(queryClient.invalidateQueries({ queryKey: noteKeys.comments(parentNoteName) }));
        invalidationPromises.push(queryClient.invalidateQueries({ queryKey: noteKeys.detail(parentNoteName) }));
      }
      // Hosts that close after saving (edit, comment) hold a brief "Saved"
      // confirmation on the toolbar while the caches refresh underneath. The
      // in-place composer clears immediately, so it shows nothing.
      if (noteName || parentNoteName) {
        dispatch(actions.setLoading("saving", false));
        dispatch(actions.setJustSaved(true));
        invalidationPromises.push(new Promise((resolve) => setTimeout(resolve, SAVED_CONFIRMATION_MS)));
      }
      await Promise.all(invalidationPromises);

      dispatch(actions.reset());
      if (!noteName && defaultVisibility) {
        dispatch(actions.setMetadata({ visibility: defaultVisibility }));
      }
      // Reset creates a fresh editor state, so restore calendar-derived values
      // for the next note created without remounting this composer.
      if (!noteName && defaultCreateTime) {
        dispatch(actions.setTimestamps({ createTime: defaultCreateTime, updateTime: defaultCreateTime }));
      }

      if (!noteName && !parentNoteName) {
        markNewNote(result.noteName);
      } else if (noteName) {
        markNewNote((currentName) => (currentName === noteName ? null : currentName));
      }
      onConfirm?.(result.noteName);
    } catch (error) {
      keepUploadedFiles();
      if (uploaded.size > 0) {
        // The uploads landed even though the save did not.
        void queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() });
        void queryClient.invalidateQueries({ queryKey: userKeys.stats() });
      }
      handleError(error, toast.error, {
        context: "Failed to save note",
        fallbackMessage: errorService.getErrorMessage(error),
      });
    } finally {
      dispatch(actions.setLoading("saving", false));
      dispatch(actions.setJustSaved(false));
    }
  }, [
    actions,
    defaultCreateTime,
    defaultSpace,
    defaultVisibility,
    discardDraft,
    dispatch,
    getState,
    markNewNote,
    noteName,
    onCancel,
    onConfirm,
    parentNoteName,
    queryClient,
    t,
  ]);
}

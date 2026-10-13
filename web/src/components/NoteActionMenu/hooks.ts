import { useQueryClient } from "@tanstack/react-query";
import copy from "copy-to-clipboard";
import { useCallback } from "react";
import toast from "react-hot-toast";
import { useLocation } from "react-router-dom";
import { useInstance } from "@/contexts/InstanceContext";
import useNavigateTo from "@/hooks/useNavigateTo";
import { noteKeys, useDeleteNote, useUpdateNote } from "@/hooks/useNoteQueries";
import { userKeys } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import { extractNoteIdFromName } from "@/lib/resource-names";
import { ROUTES } from "@/router/routes";
import { State } from "@/types/proto/api/common_pb";
import type { Note } from "@/types/proto/api/note_service_pb";
import { useTranslate } from "@/utils/i18n";
import { checkAllTasks, uncheckAllTasks } from "@/utils/markdown-task-actions";
import { isNoteDetailPath } from "../NoteView/navigation";

interface UseNoteActionHandlersOptions {
  note: Note;
  parentPage?: string;
  onEdit?: () => void;
  setDeleteDialogOpen: (open: boolean) => void;
}

export const useNoteActionHandlers = ({ note, parentPage, onEdit, setDeleteDialogOpen }: UseNoteActionHandlersOptions) => {
  const t = useTranslate();
  const location = useLocation();
  const navigateTo = useNavigateTo();
  const queryClient = useQueryClient();
  const { profile } = useInstance();
  const { mutateAsync: updateNote } = useUpdateNote();
  const { mutateAsync: deleteNote } = useDeleteNote();
  const isInNoteDetailPage = isNoteDetailPath(location.pathname, note.name);

  const noteUpdatedCallback = useCallback(() => {
    // Invalidate user stats to trigger refetch
    queryClient.invalidateQueries({ queryKey: userKeys.stats() });
  }, [queryClient]);

  const updateNoteContent = useCallback(
    async (nextContent: string, context: string) => {
      if (nextContent === note.content) {
        return;
      }

      try {
        await updateNote({
          update: {
            name: note.name,
            content: nextContent,
          },
          updateMask: ["content", "update_time"],
        });
        toast.success(t("note.task-actions.updated"));
      } catch (error: unknown) {
        handleError(error, toast.error, {
          context,
          fallbackMessage: "An error occurred",
        });
      }
    },
    [note.content, note.name, t, updateNote],
  );

  const handleTogglePinNoteBtnClick = useCallback(async () => {
    try {
      await updateNote({
        update: {
          name: note.name,
          pinned: !note.pinned,
        },
        updateMask: ["pinned"],
      });
    } catch {
      // do nothing
    }
  }, [note.name, note.pinned, updateNote]);

  const handleEditNoteClick = useCallback(() => {
    onEdit?.();
  }, [onEdit]);

  const handleToggleNoteStatusClick = useCallback(async () => {
    const isArchiving = note.state !== State.ARCHIVED;
    const state = note.state === State.ARCHIVED ? State.NORMAL : State.ARCHIVED;
    const message = note.state === State.ARCHIVED ? t("message.restored-successfully") : t("message.archived-successfully");

    try {
      await updateNote({
        update: {
          name: note.name,
          state,
        },
        updateMask: ["state"],
      });
      toast.success(message);
    } catch (error: unknown) {
      handleError(error, toast.error, {
        context: `${isArchiving ? "Archive" : "Restore"} note`,
        fallbackMessage: "An error occurred",
      });
      return;
    }

    if (isInNoteDetailPage) {
      navigateTo(ROUTES.HOME);
    }
    noteUpdatedCallback();
  }, [note.name, note.state, t, isInNoteDetailPage, navigateTo, noteUpdatedCallback, updateNote]);

  const handleCopyLink = useCallback(() => {
    let host = profile.instanceUrl;
    if (host === "") {
      host = window.location.origin;
    }
    // A comment's link lands on the comment inside its parent note, as the comment previews do.
    const path = note.parent ? `${note.parent}#${extractNoteIdFromName(note.name)}` : note.name;
    copy(`${host}/${path}`);
    toast.success(t("message.succeed-copy-link"));
  }, [note.name, note.parent, t, profile.instanceUrl]);

  const handleCopyContent = useCallback(() => {
    copy(note.content);
    toast.success(t("message.succeed-copy-content"));
  }, [note.content, t]);

  const handleCheckAllTaskListItemsClick = useCallback(async () => {
    await updateNoteContent(checkAllTasks(note.content), "Check note task list items");
  }, [note.content, updateNoteContent]);

  const handleUncheckAllTaskListItemsClick = useCallback(async () => {
    await updateNoteContent(uncheckAllTasks(note.content), "Uncheck note task list items");
  }, [note.content, updateNoteContent]);

  const handleDeleteNoteClick = useCallback(() => {
    setDeleteDialogOpen(true);
  }, [setDeleteDialogOpen]);

  const confirmDeleteNote = useCallback(async () => {
    try {
      await deleteNote(note.name);
    } catch (error: unknown) {
      handleError(error, toast.error, { context: "Delete note", fallbackMessage: "An error occurred" });
      return;
    }
    toast.success(t("message.deleted-successfully"));
    if (note.parent) {
      queryClient.invalidateQueries({ queryKey: noteKeys.comments(note.parent) });
      queryClient.invalidateQueries({ queryKey: noteKeys.detail(note.parent) });
    }
    if (isInNoteDetailPage) {
      navigateTo(parentPage || ROUTES.HOME);
    }
    noteUpdatedCallback();
  }, [note.name, note.parent, t, isInNoteDetailPage, parentPage, navigateTo, noteUpdatedCallback, deleteNote, queryClient]);

  return {
    isInNoteDetailPage,
    handleTogglePinNoteBtnClick,
    handleEditNoteClick,
    handleToggleNoteStatusClick,
    handleCopyLink,
    handleCopyContent,
    handleCheckAllTaskListItemsClick,
    handleUncheckAllTaskListItemsClick,
    handleDeleteNoteClick,
    confirmDeleteNote,
  };
};

import { ArchiveRestoreIcon, CheckCheckIcon, Edit3Icon, ListRestartIcon, MoreVerticalIcon, TrashIcon } from "lucide-react";
import { useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { State } from "@/types/proto/api/common_pb";
import { useTranslate } from "@/utils/i18n";
import { useNoteActionHandlers } from "./hooks";
import type { NoteActionMenuProps } from "./types";

/**
 * The note's action menu for its author: write on it (Edit, the task action), then
 * Delete last. An archived note offers only Restore and Delete, and a viewer gets no menu.
 */
const NoteActionMenu = (props: NoteActionMenuProps) => {
  const { note, parentPage, readonly } = props;
  const t = useTranslate();

  // Dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // Derived state
  const isArchived = note.state === State.ARCHIVED;
  const hasTaskList = Boolean(note.property?.hasTaskList);
  const hasOpenTasks = Boolean(note.property?.hasIncompleteTasks);

  // Action handlers
  const {
    handleEditNoteClick,
    handleToggleNoteStatusClick,
    handleCheckAllTaskListItemsClick,
    handleUncheckAllTaskListItemsClick,
    handleDeleteNoteClick,
    confirmDeleteNote,
  } = useNoteActionHandlers({
    note,
    parentPage,
    onEdit: props.onEdit,
    setDeleteDialogOpen,
  });

  const deleteItem = (
    <DropdownMenuItem variant="destructive" onClick={handleDeleteNoteClick}>
      <TrashIcon />
      {t("common.delete")}
    </DropdownMenuItem>
  );

  const renderItems = () => {
    if (isArchived) {
      return (
        <>
          <DropdownMenuItem onClick={handleToggleNoteStatusClick}>
            <ArchiveRestoreIcon />
            {t("common.restore")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {deleteItem}
        </>
      );
    }

    return (
      <>
        <DropdownMenuItem onClick={handleEditNoteClick}>
          <Edit3Icon />
          {t("common.edit")}
        </DropdownMenuItem>
        {/* One task action at a time: finish the open tasks, or reset a finished list. */}
        {hasTaskList &&
          (hasOpenTasks ? (
            <DropdownMenuItem onClick={handleCheckAllTaskListItemsClick}>
              <CheckCheckIcon />
              {t("note.task-actions.check-all")}
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem onClick={handleUncheckAllTaskListItemsClick}>
              <ListRestartIcon />
              {t("note.task-actions.uncheck-all")}
            </DropdownMenuItem>
          ))}

        <DropdownMenuSeparator />
        {deleteItem}
      </>
    );
  };

  // A viewer has nothing to do with someone else's note.
  if (readonly) return null;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="quiet" size="icon-sm" aria-label={t("common.more")} />}>
        <MoreVerticalIcon className="size-4" strokeWidth={1.8} />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={2} size="sm">
        {renderItems()}
      </DropdownMenuContent>

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title={t("note.delete-confirm")}
        confirmLabel={t("common.delete")}
        description={t("note.delete-confirm-description")}
        cancelLabel={t("common.cancel")}
        onConfirm={confirmDeleteNote}
        confirmVariant="destructive"
      />
    </DropdownMenu>
  );
};

export default NoteActionMenu;

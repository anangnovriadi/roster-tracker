import { useRef } from "react";
import { Checkbox } from "@/components/ui/checkbox";
import { useUpdateNote } from "@/hooks/useNoteQueries";
import { toggleTaskAtIndex } from "@/utils/markdown-manipulation";
import { useNoteViewContext, useNoteViewDerived } from "../NoteView/NoteViewContext";
import { TASK_LIST_ITEM_CLASS } from "./constants";
import type { ReactMarkdownProps } from "./markdown/types";

interface TaskListItemProps extends React.InputHTMLAttributes<HTMLInputElement>, ReactMarkdownProps {
  checked?: boolean;
}

export const TaskListItem: React.FC<TaskListItemProps> = ({ checked, node: _node, ...props }) => {
  const { note } = useNoteViewContext();
  const { readonly } = useNoteViewDerived();
  const checkboxRef = useRef<HTMLButtonElement>(null);
  const { mutate: updateNote } = useUpdateNote();

  const handleChange = async (newChecked: boolean) => {
    // Don't update if readonly or no note
    if (readonly || !note) {
      return;
    }

    // Find the task index by walking up the DOM
    const listItem = checkboxRef.current?.closest("li.task-list-item");
    if (!listItem) {
      return;
    }

    // Get task index from data attribute, or calculate by counting
    const taskIndexStr = listItem.getAttribute("data-task-index");
    let taskIndex = 0;

    if (taskIndexStr !== null) {
      taskIndex = parseInt(taskIndexStr);
    } else {
      // Fallback: Calculate index by counting all task list items in the entire note
      // We need to search from the root note content container, not just the nearest list
      // to ensure nested tasks are counted in document order
      let searchRoot = listItem.closest("[data-note-content]");

      // If note content container not found, search from document body
      if (!searchRoot) {
        searchRoot = document.body;
      }

      const allTaskItems = searchRoot.querySelectorAll(`li.${TASK_LIST_ITEM_CLASS}`);
      for (let i = 0; i < allTaskItems.length; i++) {
        if (allTaskItems[i] === listItem) {
          taskIndex = i;
          break;
        }
      }
    }

    // Update note content using the string manipulation utility
    const newContent = toggleTaskAtIndex(note.content, taskIndex, newChecked);
    updateNote({
      update: {
        name: note.name,
        content: newContent,
      },
      updateMask: ["content", "update_time"],
    });
  };

  // Override the disabled prop from remark-gfm (which defaults to true).
  // `checked` is undefined for unchecked tasks; Base UI locks controlled/uncontrolled
  // mode at mount, so coerce to boolean to keep the checkbox always controlled.
  return (
    <Checkbox ref={checkboxRef} checked={Boolean(checked)} disabled={readonly} onCheckedChange={handleChange} className={props.className} />
  );
};

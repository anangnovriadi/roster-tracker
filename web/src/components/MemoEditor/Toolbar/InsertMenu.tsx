import { CheckIcon, ImageIcon, LoaderIcon, Maximize2Icon, PaperclipIcon, PlusIcon, TypeIcon } from "lucide-react";
import { useCallback, useRef } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useTranslate } from "@/utils/i18n";
import { useFileUpload } from "../hooks";
import { useEditorContext } from "../state";
import type { InsertMenuProps } from "../types";
import type { LocalFile } from "../types/attachment";

const InsertMenu = (props: InsertMenuProps) => {
  const t = useTranslate();
  const { actions, dispatch, getState } = useEditorContext();
  const { viewToggles, isUploading: isUploadingProp } = props;
  const inlineImageInputRef = useRef<HTMLInputElement>(null);

  const { fileInputRef, selectingFlag, handleFileInputChange, handleUploadClick } = useFileUpload((newFiles: LocalFile[]) => {
    if (getState().ui.isLoading.saving) return;
    newFiles.forEach((file) => dispatch(actions.addLocalFile(file)));
  });

  const isUploading = selectingFlag || isUploadingProp;
  const insertionDisabled = isUploading || props.isSaving;

  const handleAttachmentUploadClick = useCallback(() => {
    if (getState().ui.isLoading.saving) return;
    handleUploadClick();
  }, [getState, handleUploadClick]);

  const handleInlineImageUploadClick = useCallback(() => {
    if (getState().ui.isLoading.saving) return;
    inlineImageInputRef.current?.click();
  }, [getState]);

  const handleInlineImageInputChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files ?? []);
      if (files.length > 0) props.onInsertImages(files);
      event.target.value = "";
    },
    [props.onInsertImages],
  );

  // Insert actions (add content).
  const insertItems = [
    { key: "attachment", label: t("editor.insert-menu.add-attachment"), icon: PaperclipIcon, onClick: handleAttachmentUploadClick },
    { key: "inline-image", label: t("editor.insert-menu.insert-image"), icon: ImageIcon, onClick: handleInlineImageUploadClick },
  ];

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button variant="outline" size="icon-compact" disabled={insertionDisabled} aria-label={t("common.add")} />}
        >
          {isUploading ? (
            <LoaderIcon className="size-4 animate-spin" strokeWidth={1.8} />
          ) : (
            <PlusIcon className="size-4" strokeWidth={1.8} />
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" size="sm">
          {insertItems.map((item) => (
            <DropdownMenuItem key={item.key} onClick={item.onClick} disabled={props.isSaving}>
              <item.icon />
              {item.label}
            </DropdownMenuItem>
          ))}
          {/* View toggles: focus mode + formatting-toolbar visibility. Absent
              when a host owns the editor's presentation — neither applies there. */}
          {viewToggles && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={viewToggles.onToggleFocusMode}>
                <Maximize2Icon />
                {t("editor.focus-mode")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={viewToggles.onToggleFormattingToolbar}>
                <TypeIcon />
                {t("editor.formatting-toolbar")}
                {viewToggles.isFormattingToolbarVisible && <CheckIcon className="ms-auto size-3.5" />}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Hidden file input */}
      <input
        className="hidden"
        ref={fileInputRef}
        disabled={insertionDisabled}
        onChange={handleFileInputChange}
        type="file"
        multiple={true}
        accept=""
      />
      <input
        className="hidden"
        ref={inlineImageInputRef}
        disabled={insertionDisabled}
        onChange={handleInlineImageInputChange}
        type="file"
        multiple={true}
        accept="image/*"
      />
    </>
  );
};

export default InsertMenu;

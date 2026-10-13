import { describe, expect, test } from "vitest";
import { validationService } from "@/components/NoteEditor/services/validationService";
import { createInitialState } from "@/components/NoteEditor/state";

describe("inline image editor validation", () => {
  test("blocks saving while an inline image insertion is unresolved", () => {
    const state = createInitialState();
    state.content = "note";
    state.ui.pendingInlineImageInsertions = 1;

    expect(validationService.canSave(state)).toEqual({
      valid: false,
      reason: "editor.validation.resolve-image-uploads",
    });
  });

  test("allows saving after the insertion is resolved", () => {
    const state = createInitialState();
    state.content = "note";

    expect(validationService.canSave(state)).toEqual({ valid: true });
  });
});

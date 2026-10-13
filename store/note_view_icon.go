package store

import (
	"github.com/pkg/errors"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
)

// ValidateNoteViewIcon validates optional display metadata for a saved view.
func ValidateNoteViewIcon(icon *storepb.NoteViewsUserSetting_NoteView_Icon) error {
	if icon == nil {
		return nil
	}
	switch value := icon.Value.(type) {
	case *storepb.NoteViewsUserSetting_NoteView_Icon_Emoji:
		return validateEmojiIcon(value.Emoji)
	case *storepb.NoteViewsUserSetting_NoteView_Icon_Lucide:
		return validateLucideIcon(value.Lucide)
	default:
		return errors.New("view icon must specify emoji or lucide")
	}
}

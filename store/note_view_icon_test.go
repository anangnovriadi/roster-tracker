package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
)

func TestValidateNoteViewIcon(t *testing.T) {
	require.NoError(t, ValidateNoteViewIcon(nil))
	require.Error(t, ValidateNoteViewIcon(&storepb.NoteViewsUserSetting_NoteView_Icon{}))
	for _, emoji := range []string{"🌱", "👩🏽‍🌾", "🇸🇬", "❤️", "1️⃣"} {
		require.NoError(t, ValidateNoteViewIcon(&storepb.NoteViewsUserSetting_NoteView_Icon{
			Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Emoji{Emoji: emoji},
		}), emoji)
	}
	for _, emoji := range []string{"", "leaf", "🌱🌱", " 🌱", "🏻", "\xff"} {
		require.Error(t, ValidateNoteViewIcon(&storepb.NoteViewsUserSetting_NoteView_Icon{
			Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Emoji{Emoji: emoji},
		}), emoji)
	}
	for _, name := range []string{"leaf", "book-open", "circle-1", "future-icon", strings.Repeat("a", 128)} {
		require.NoError(t, ValidateNoteViewIcon(&storepb.NoteViewsUserSetting_NoteView_Icon{
			Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Lucide{Lucide: name},
		}), name)
	}
	for _, name := range []string{"", "LeafIcon", "book_open", "-leaf", "leaf-", "book--open", "<svg>", strings.Repeat("a", 129)} {
		require.Error(t, ValidateNoteViewIcon(&storepb.NoteViewsUserSetting_NoteView_Icon{
			Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Lucide{Lucide: name},
		}), name)
	}
}

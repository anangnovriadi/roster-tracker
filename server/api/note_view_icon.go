package api

import (
	apipb "github.com/anangnovriadi/roster-tracker/proto/gen/api"
	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
)

func convertNoteViewIconFromStore(icon *storepb.NoteViewsUserSetting_NoteView_Icon) *apipb.NoteView_Icon {
	if icon == nil {
		return nil
	}
	switch value := icon.Value.(type) {
	case *storepb.NoteViewsUserSetting_NoteView_Icon_Emoji:
		return &apipb.NoteView_Icon{Value: &apipb.NoteView_Icon_Emoji{Emoji: value.Emoji}}
	case *storepb.NoteViewsUserSetting_NoteView_Icon_Lucide:
		return &apipb.NoteView_Icon{Value: &apipb.NoteView_Icon_Lucide{Lucide: value.Lucide}}
	default:
		return nil
	}
}

func convertNoteViewIconToStore(icon *apipb.NoteView_Icon) *storepb.NoteViewsUserSetting_NoteView_Icon {
	if icon == nil {
		return nil
	}
	switch value := icon.Value.(type) {
	case *apipb.NoteView_Icon_Emoji:
		return &storepb.NoteViewsUserSetting_NoteView_Icon{Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Emoji{Emoji: value.Emoji}}
	case *apipb.NoteView_Icon_Lucide:
		return &storepb.NoteViewsUserSetting_NoteView_Icon{Value: &storepb.NoteViewsUserSetting_NoteView_Icon_Lucide{Lucide: value.Lucide}}
	default:
		// Preserve an empty icon so validation rejects it instead of treating it as a reset.
		return &storepb.NoteViewsUserSetting_NoteView_Icon{}
	}
}

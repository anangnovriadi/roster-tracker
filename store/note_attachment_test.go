package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anangnovriadi/roster-tracker/store"
)

func TestNoteMutationRejectsInvalidInputBeforeWriting(t *testing.T) {
	creation := func() *store.NoteMutation {
		return &store.NoteMutation{NoteCreate: &store.Note{UID: "new-note", CreatorID: 1, Visibility: store.Private}}
	}
	references := func(relations ...*store.NoteRelation) *store.NoteMutation {
		return &store.NoteMutation{NoteID: 1, NoteCreatorID: 1, ReplaceReferenceRelations: true, ReferenceRelations: relations}
	}
	tests := []struct {
		name     string
		mutation func() *store.NoteMutation
		message  string
	}{
		{"nil mutation", func() *store.NoteMutation { return nil }, "note mutation is required"},
		{"create and update", func() *store.NoteMutation {
			m := creation()
			m.NoteUpdate = &store.UpdateNote{ID: 1}
			return m
		}, "cannot update or remove existing state"},
		{"create with update policy", func() *store.NoteMutation {
			m := creation()
			m.Policy = &store.NoteWritePolicy{ActorUserID: 1}
			return m
		}, "cannot update or remove existing state"},
		{"create and delete attachment", func() *store.NoteMutation {
			m := creation()
			m.RemovedAttachmentIDs = []int32{1}
			return m
		}, "cannot update or remove existing state"},
		{"missing creator", func() *store.NoteMutation {
			m := creation()
			m.NoteCreate.CreatorID = 0
			return m
		}, "requires creator"},
		{"invalid creation UID", func() *store.NoteMutation {
			m := creation()
			m.NoteCreate.UID = "invalid uid"
			return m
		}, "invalid uid"},
		{"invalid creation visibility", func() *store.NoteMutation {
			m := creation()
			m.NoteCreate.Visibility = "INVALID"
			return m
		}, "invalid visibility"},
		{"space audience without placement", func() *store.NoteMutation {
			m := creation()
			m.NoteCreate.Visibility = store.SpaceAudience
			return m
		}, "SPACE visibility requires a space"},
		{"invalid comment context", func() *store.NoteMutation {
			m := creation()
			m.CommentContextNoteID = new(int32(0))
			return m
		}, "valid context note"},
		{"missing policy actor", func() *store.NoteMutation {
			return &store.NoteMutation{Policy: &store.NoteWritePolicy{}}
		}, "write policy requires actor"},
		{"missing fallback policy actor", func() *store.NoteMutation {
			return &store.NoteMutation{NoteUpdate: &store.UpdateNote{ID: 1, Policy: &store.NoteWritePolicy{}}}
		}, "write policy requires actor"},
		{"conflicting policy actions", func() *store.NoteMutation {
			return &store.NoteMutation{Policy: &store.NoteWritePolicy{ActorUserID: 1, LifecycleOnly: true, CreatingShare: true}}
		}, "conflicting actions"},
		{"conflicting fallback policy actions", func() *store.NoteMutation {
			return &store.NoteMutation{NoteUpdate: &store.UpdateNote{ID: 1, Policy: &store.NoteWritePolicy{
				ActorUserID: 1, LifecycleOnly: true, CreatingShare: true,
			}}}
		}, "conflicting actions"},
		{"zero removed ID", func() *store.NoteMutation {
			return &store.NoteMutation{RemovedAttachmentIDs: []int32{0}}
		}, "IDs must be positive"},
		{"nil attachment binding", func() *store.NoteMutation {
			return &store.NoteMutation{Bindings: []*store.NoteAttachmentBinding{nil}}
		}, "attachment binding is required"},
		{"negative removed ID", func() *store.NoteMutation {
			return &store.NoteMutation{RemovedAttachmentIDs: []int32{-1}}
		}, "IDs must be positive"},
		{"nonadjacent duplicate removed IDs", func() *store.NoteMutation {
			return &store.NoteMutation{RemovedAttachmentIDs: []int32{2, 1, 2}}
		}, "duplicate removed attachment ID"},
		{"bind and remove same attachment", func() *store.NoteMutation {
			return &store.NoteMutation{RemovedAttachmentIDs: []int32{1}, Bindings: []*store.NoteAttachmentBinding{{ID: 1}}}
		}, "both bound and removed"},
		{"require and remove same attachment", func() *store.NoteMutation {
			return &store.NoteMutation{RemovedAttachmentIDs: []int32{1}, RequiredAttachmentIDs: []int32{1}}
		}, "both required and removed"},
		{"reference without actor", func() *store.NoteMutation {
			m := references()
			m.NoteCreatorID = 0
			return m
		}, "requires an actor"},
		{"nil reference", func() *store.NoteMutation { return references(nil) }, "only REFERENCE"},
		{"invalid reference target", func() *store.NoteMutation {
			return references(&store.NoteRelation{NoteID: 1, Type: store.NoteRelationReference})
		}, "only REFERENCE"},
		{"comment relation replacement", func() *store.NoteMutation {
			return references(&store.NoteRelation{NoteID: 1, RelatedNoteID: 2, Type: store.NoteRelationComment})
		}, "only REFERENCE"},
		{"wrong reference source", func() *store.NoteMutation {
			return references(&store.NoteRelation{NoteID: 3, RelatedNoteID: 2, Type: store.NoteRelationReference})
		}, "source does not match"},
		{"self reference", func() *store.NoteMutation {
			return references(&store.NoteRelation{NoteID: 1, RelatedNoteID: 1, Type: store.NoteRelationReference})
		}, "reflexive note relations"},
		{"duplicate references", func() *store.NoteMutation {
			return references(
				&store.NoteRelation{NoteID: 1, RelatedNoteID: 2, Type: store.NoteRelationReference},
				&store.NoteRelation{NoteID: 1, RelatedNoteID: 2, Type: store.NoteRelationReference},
			)
		}, "duplicate note reference"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// No driver is installed: invalid input must fail before database access.
			s := &store.Store{}
			require.ErrorContains(t, s.ApplyNoteMutation(context.Background(), tc.mutation()), tc.message)
		})
	}
}

func TestNoteLifecycleMutationRejectsDependentWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*store.NoteMutation)
	}{
		{"bindings", func(m *store.NoteMutation) { m.Bindings = []*store.NoteAttachmentBinding{{ID: 1}} }},
		{"removals", func(m *store.NoteMutation) { m.RemovedAttachmentIDs = []int32{1} }},
		{"required attachments", func(m *store.NoteMutation) { m.RequiredAttachmentIDs = []int32{1} }},
		{"reference replacement", func(m *store.NoteMutation) { m.ReplaceReferenceRelations = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, source := range []string{"mutation policy", "update policy fallback"} {
				t.Run(source, func(t *testing.T) {
					mutation := &store.NoteMutation{
						NoteID: 1, NoteCreatorID: 1,
						NoteUpdate: &store.UpdateNote{ID: 1, ClearSpace: true},
					}
					policy := &store.NoteWritePolicy{ActorUserID: 1, LifecycleOnly: true}
					if source == "mutation policy" {
						mutation.Policy = policy
					} else {
						mutation.NoteUpdate.Policy = policy
					}
					tc.change(mutation)
					require.ErrorIs(t, (&store.Store{}).ApplyNoteMutation(context.Background(), mutation), store.ErrNoteSpaceMembershipRequired)
				})
			}
		})
	}
}

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

type noteMutationFixture struct {
	t                    *testing.T
	ctx                  context.Context
	store                *store.Store
	owner, peer          *store.User
	note, target, next   *store.Note
	added, removed, kept *store.Attachment
}

func newNoteMutationFixture(t *testing.T) *noteMutationFixture {
	t.Helper()
	f := &noteMutationFixture{t: t, ctx: context.Background()}
	f.store = NewTestingStore(f.ctx, t)
	t.Cleanup(func() { require.NoError(t, f.store.Close()) })
	var err error
	f.owner, err = f.store.CreateUser(f.ctx, &store.User{Username: "mutation-owner", Role: store.RoleUser})
	require.NoError(t, err)
	f.peer, err = f.store.CreateUser(f.ctx, &store.User{Username: "mutation-peer", Role: store.RoleUser})
	require.NoError(t, err)
	for i, dest := range []**store.Note{&f.note, &f.target, &f.next} {
		uid := []string{"mutation-source", "mutation-target", "mutation-next"}[i]
		*dest, err = f.store.CreateNote(f.ctx, &store.Note{
			UID: uid, CreatorID: f.owner.ID, Content: uid, Visibility: store.Private,
		})
		require.NoError(t, err)
	}
	for i, dest := range []**store.Attachment{&f.added, &f.removed, &f.kept} {
		uid := []string{"mutation-added", "mutation-removed", "mutation-kept"}[i]
		attachment := &store.Attachment{UID: uid, CreatorID: f.owner.ID, Filename: uid + ".txt", Type: "text/plain", Blob: []byte(uid)}
		if i > 0 {
			attachment.NoteID = &f.note.ID
		}
		*dest, err = f.store.CreateAttachment(f.ctx, attachment)
		require.NoError(t, err)
	}
	_, err = f.store.UpsertNoteRelation(f.ctx, &store.NoteRelation{
		NoteID: f.note.ID, RelatedNoteID: f.target.ID, Type: store.NoteRelationReference,
	})
	require.NoError(t, err)
	return f
}

func (f *noteMutationFixture) mutation() *store.NoteMutation {
	return &store.NoteMutation{
		NoteID: f.note.ID, NoteCreatorID: f.owner.ID, ExpectedNoteContent: f.note.Content,
		NoteUpdate: &store.UpdateNote{ID: f.note.ID, Content: new("updated content")},
		Bindings: []*store.NoteAttachmentBinding{
			{ID: f.added.ID, UID: f.added.UID, UpdatedTs: 1600000000},
		},
		RemovedAttachmentIDs:      []int32{f.removed.ID},
		RequiredAttachmentIDs:     []int32{f.kept.ID, f.added.ID},
		ReplaceReferenceRelations: true,
		ReferenceRelations: []*store.NoteRelation{
			{NoteID: f.note.ID, RelatedNoteID: f.next.ID, Type: store.NoteRelationReference},
		},
	}
}

type noteMutationState struct {
	notes       []*store.Note
	attachments []*store.Attachment
	relations   []*store.NoteRelation
}

func (f *noteMutationFixture) state() noteMutationState {
	f.t.Helper()
	notes, err := f.store.ListNotes(f.ctx, &store.FindNote{})
	require.NoError(f.t, err)
	attachments, err := f.store.ListAttachments(f.ctx, &store.FindAttachment{GetBlob: true, SkipDefaultLimit: true})
	require.NoError(f.t, err)
	relations, err := f.store.ListNoteRelations(f.ctx, &store.FindNoteRelation{})
	require.NoError(f.t, err)
	return noteMutationState{notes: notes, attachments: attachments, relations: relations}
}

func TestNoteMutationRevalidatesPreparedState(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*noteMutationFixture, *store.NoteMutation)
		message string
	}{
		{"note content changed", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.UpdateNote(f.ctx, &store.UpdateNote{ID: f.note.ID, Content: new("concurrent edit")}))
		}, "note changed"},
		{"note creator mismatch", func(f *noteMutationFixture, m *store.NoteMutation) {
			m.NoteCreatorID = f.peer.ID
			m.ReplaceReferenceRelations = false
		}, "note changed"},
		{"note deleted", func(f *noteMutationFixture, m *store.NoteMutation) {
			m.ReplaceReferenceRelations = false
			require.NoError(f.t, f.store.DeleteNote(f.ctx, &store.DeleteNote{ID: f.note.ID}))
		}, "note no longer exists"},
		{"removed attachment deleted", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.GetDriver().DeleteAttachment(f.ctx, &store.DeleteAttachment{ID: f.removed.ID}))
		}, "removed attachment no longer exists"},
		{"removed attachment moved", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.UpdateAttachment(f.ctx, &store.UpdateAttachment{ID: f.removed.ID, NoteID: &f.target.ID}))
		}, "no longer removable"},
		{"new binding deleted", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.GetDriver().DeleteAttachment(f.ctx, &store.DeleteAttachment{ID: f.added.ID}))
		}, "no longer exists"},
		{"new binding taken by another note", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.UpdateAttachment(f.ctx, &store.UpdateAttachment{ID: f.added.ID, NoteID: &f.target.ID}))
		}, "no longer available"},
		{"existing binding moved after preparation", func(f *noteMutationFixture, m *store.NoteMutation) {
			m.Bindings = append(m.Bindings, &store.NoteAttachmentBinding{ID: f.kept.ID, UID: f.kept.UID, WasBoundToNote: true})
			require.NoError(f.t, f.store.UpdateAttachment(f.ctx, &store.UpdateAttachment{ID: f.kept.ID, NoteID: &f.target.ID}))
		}, "no longer bound"},
		{"claimed existing binding is unbound", func(_ *noteMutationFixture, m *store.NoteMutation) {
			m.Bindings[0].WasBoundToNote = true
		}, "no longer bound"},
		{"required attachment moved after preparation", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.UpdateAttachment(f.ctx, &store.UpdateAttachment{ID: f.kept.ID, NoteID: &f.target.ID}))
		}, "referenced attachment is no longer bound"},
		{"reference target deleted", func(f *noteMutationFixture, _ *store.NoteMutation) {
			require.NoError(f.t, f.store.DeleteNote(f.ctx, &store.DeleteNote{ID: f.next.ID}))
		}, "note state changed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			mutation := f.mutation()
			tc.change(f, mutation)
			before := f.state()
			err := f.store.ApplyNoteMutation(f.ctx, mutation)
			require.ErrorIs(t, err, store.ErrNoteMutationConflict)
			require.ErrorContains(t, err, tc.message)
			require.Equal(t, before, f.state(), "rejected mutation must preserve the current database state")
		})
	}
}

func TestNoteMutationRollsBackAfterAttachmentWrites(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*noteMutationFixture, *store.NoteMutation)
	}{
		{"update target mismatch", func(f *noteMutationFixture, m *store.NoteMutation) { m.NoteUpdate.ID = f.target.ID }},
		{"note UID uniqueness failure", func(f *noteMutationFixture, m *store.NoteMutation) { m.NoteUpdate.UID = &f.target.UID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			mutation := f.mutation()
			tc.change(f, mutation)
			before := f.state()
			// Both failures occur after binding the added attachment and deleting
			// the removed attachment, so this checks a real transaction rollback.
			require.Error(t, f.store.ApplyNoteMutation(f.ctx, mutation))
			require.Equal(t, before, f.state())
			// A fresh valid write also proves the failed transaction released its locks.
			require.NoError(t, f.store.ApplyNoteMutation(f.ctx, f.mutation()))
		})
	}
}

func TestNoteMutationRollsBackAfterReferenceInsertFailure(t *testing.T) {
	f := newNoteMutationFixture(t)
	first, err := f.store.CreateNote(f.ctx, &store.Note{
		UID: "first-replacement", CreatorID: f.owner.ID, Content: "replacement", Visibility: store.Private,
	})
	require.NoError(t, err)
	// Inject a database error after note fields, attachment bindings/removals,
	// and the first replacement reference have already been written.
	var statements []string
	drop := "DROP TRIGGER reject_mutation_reference"
	switch getDriverFromEnv() {
	case "sqlite":
		statements = []string{fmt.Sprintf(`CREATE TRIGGER reject_mutation_reference
			BEFORE INSERT ON note_relation WHEN NEW.related_note_id = %d
			BEGIN SELECT RAISE(ABORT, 'mutation test reference failure'); END`, f.next.ID)}
	case "mysql":
		statements = []string{fmt.Sprintf(`CREATE TRIGGER reject_mutation_reference
			BEFORE INSERT ON note_relation FOR EACH ROW BEGIN
			IF NEW.related_note_id = %d THEN
				SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'mutation test reference failure';
			END IF; END`, f.next.ID)}
	case "postgres":
		statements = []string{
			fmt.Sprintf(`CREATE FUNCTION reject_mutation_reference() RETURNS trigger AS $$
			BEGIN IF NEW.related_note_id = %d THEN
				RAISE EXCEPTION 'mutation test reference failure';
			END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`, f.next.ID),
			`CREATE TRIGGER reject_mutation_reference BEFORE INSERT ON note_relation
			FOR EACH ROW EXECUTE FUNCTION reject_mutation_reference()`,
		}
		drop += " ON note_relation"
	default:
		t.Fatalf("unsupported driver: %s", getDriverFromEnv())
	}
	for _, statement := range statements {
		_, err := f.store.GetDriver().GetDB().ExecContext(f.ctx, statement)
		require.NoError(t, err)
	}
	before := f.state()
	mutation := f.mutation()
	mutation.ReferenceRelations = append([]*store.NoteRelation{
		{NoteID: f.note.ID, RelatedNoteID: first.ID, Type: store.NoteRelationReference},
	}, mutation.ReferenceRelations...)
	require.ErrorContains(t, f.store.ApplyNoteMutation(f.ctx, mutation), "mutation test reference failure")
	require.Equal(t, before, f.state(), "all three tables must roll back, including the deleted original reference")
	_, err = f.store.GetDriver().GetDB().ExecContext(f.ctx, drop)
	require.NoError(t, err)
	require.NoError(t, f.store.ApplyNoteMutation(f.ctx, f.mutation()))
}

func TestNoteCreationMutationRollsBackCommentAndAttachmentBindings(t *testing.T) {
	for _, comment := range []bool{false, true} {
		name := "top level"
		if comment {
			name = "comment"
		}
		t.Run(name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			foreign, err := f.store.CreateAttachment(f.ctx, &store.Attachment{
				UID: "foreign-attachment", CreatorID: f.peer.ID, Filename: "foreign.txt", Type: "text/plain",
			})
			require.NoError(t, err)
			before := f.state()
			created := &store.Note{UID: "rejected-creation", CreatorID: f.owner.ID, Content: "new", Visibility: store.Private}
			mutation := &store.NoteMutation{
				NoteCreate: created,
				Bindings: []*store.NoteAttachmentBinding{
					{ID: f.added.ID, UID: f.added.UID, UpdatedTs: 1600000000},
					{ID: foreign.ID, UID: foreign.UID, UpdatedTs: 1600000000},
				},
				ReplaceReferenceRelations: true,
				ReferenceRelations:        []*store.NoteRelation{{RelatedNoteID: f.target.ID, Type: store.NoteRelationReference}},
			}
			if comment {
				mutation.CommentContextNoteID = &f.note.ID
			}
			require.ErrorIs(t, f.store.ApplyNoteMutation(f.ctx, mutation), store.ErrNoteMutationConflict)
			require.Equal(t, before, f.state())
			got, err := f.store.GetNote(f.ctx, &store.FindNote{UID: &created.UID})
			require.NoError(t, err)
			require.Nil(t, got, "failed creation must not leave a note or orphan COMMENT relation")
		})
	}
}

func TestNoteMutationCanceledContextDoesNotWrite(t *testing.T) {
	f := newNoteMutationFixture(t)
	before := f.state()
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	require.ErrorIs(t, f.store.ApplyNoteMutation(ctx, f.mutation()), context.Canceled)
	require.Equal(t, before, f.state())
	require.NoError(t, f.store.ApplyNoteMutation(f.ctx, f.mutation()))
}

func TestNoteMutationRejectsNilBindingWithoutPanic(t *testing.T) {
	f := newNoteMutationFixture(t)
	before := f.state()
	mutation := f.mutation()
	mutation.Bindings = append(mutation.Bindings, nil)
	var err error
	require.NotPanics(t, func() { err = f.store.ApplyNoteMutation(f.ctx, mutation) })
	require.ErrorContains(t, err, "attachment binding is required")
	require.Equal(t, before, f.state())
	require.NoError(t, f.store.ApplyNoteMutation(f.ctx, f.mutation()))
}

func TestNoteMutationConcurrentWritersCommitOnlyOneCompleteChange(t *testing.T) {
	f := newNoteMutationFixture(t)
	second, err := f.store.CreateAttachment(f.ctx, &store.Attachment{
		UID: "second-writer-attachment", CreatorID: f.owner.ID, Filename: "second.txt", Type: "text/plain",
	})
	require.NoError(t, err)
	attachments := []*store.Attachment{f.added, second}
	targets := []*store.Note{f.next, f.target}
	mutations := []*store.NoteMutation{f.mutation(), f.mutation()}
	for i, m := range mutations {
		m.NoteUpdate.Content = new(fmt.Sprintf("writer-%d", i))
		m.Bindings = []*store.NoteAttachmentBinding{{ID: attachments[i].ID, UID: attachments[i].UID, UpdatedTs: 1600000000}}
		m.RequiredAttachmentIDs = []int32{attachments[i].ID, f.kept.ID}
		m.ReferenceRelations[0].RelatedNoteID = targets[i].ID
	}
	start := make(chan struct{})
	errs := make([]error, len(mutations))
	var wg sync.WaitGroup
	for i, mutation := range mutations {
		wg.Go(func() {
			<-start
			errs[i] = f.store.ApplyNoteMutation(f.ctx, mutation)
		})
	}
	close(start)
	wg.Wait()
	winner := -1
	for i, err := range errs {
		if err == nil {
			require.Equal(t, -1, winner, "two writes based on the same content snapshot must not both commit")
			winner = i
		}
	}
	require.NotEqual(t, -1, winner, "one complete mutation should succeed: %v", errs)
	got, err := f.store.GetNote(f.ctx, &store.FindNote{ID: &f.note.ID})
	require.NoError(t, err)
	require.Equal(t, *mutations[winner].NoteUpdate.Content, got.Content)
	for i, attachment := range attachments {
		got, err := f.store.GetAttachment(f.ctx, &store.FindAttachment{ID: &attachment.ID})
		require.NoError(t, err)
		if i == winner {
			require.Equal(t, &f.note.ID, got.NoteID)
		} else {
			require.Nil(t, got.NoteID, "losing writer must not leave its attachment bound")
		}
	}
	removed, err := f.store.GetAttachment(f.ctx, &store.FindAttachment{ID: &f.removed.ID})
	require.NoError(t, err)
	require.Nil(t, removed)
	relations, err := f.store.ListNoteRelations(f.ctx, &store.FindNoteRelation{NoteID: &f.note.ID})
	require.NoError(t, err)
	require.Equal(t, mutations[winner].ReferenceRelations, relations)
}

func TestNoteMutationRechecksSpaceMembership(t *testing.T) {
	for _, policyOnUpdate := range []bool{false, true} {
		name := "mutation policy"
		if policyOnUpdate {
			name = "update policy fallback"
		}
		t.Run(name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			space, err := f.store.CreateSpace(f.ctx, &store.Space{UID: "mutation-space", Title: "Space"}, f.owner.ID)
			require.NoError(t, err)
			_, err = createSpaceMemberForTest(f.ctx, f.store, &store.SpaceMember{
				SpaceID: space.ID, UserID: f.peer.ID, Role: store.SpaceMemberRoleAdmin,
			}, f.owner.ID)
			require.NoError(t, err)
			require.NoError(t, f.store.UpdateNote(f.ctx, &store.UpdateNote{ID: f.note.ID, SpaceID: &space.ID}))
			mutation := f.mutation()
			policy := noteWritePolicy(f.owner.ID, false)
			if policyOnUpdate {
				mutation.NoteUpdate.Policy = policy
			} else {
				mutation.Policy = policy
			}
			require.NoError(t, f.store.DeleteSpaceMember(f.ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: f.owner.ID}, f.peer.ID))
			before := f.state()
			require.ErrorIs(t, f.store.ApplyNoteMutation(f.ctx, mutation), store.ErrNoteSpaceMembershipRequired)
			require.Equal(t, before, f.state())
		})
	}
}

func TestNoteMutationLifecyclePolicyFallbackRejectsDependentWrites(t *testing.T) {
	f := newNoteMutationFixture(t)
	space, err := f.store.CreateSpace(f.ctx, &store.Space{UID: "withdrawal-space", Title: "Space"}, f.owner.ID)
	require.NoError(t, err)
	require.NoError(t, f.store.UpdateNote(f.ctx, &store.UpdateNote{ID: f.note.ID, SpaceID: &space.ID}))
	mutation := f.mutation()
	mutation.NoteUpdate = &store.UpdateNote{ID: f.note.ID, ClearSpace: true, Policy: noteWritePolicy(f.owner.ID, true)}
	before := f.state()
	require.ErrorIs(t, f.store.ApplyNoteMutation(f.ctx, mutation), store.ErrNoteSpaceMembershipRequired)
	require.Equal(t, before, f.state())
}

func TestNoteMutationCommitsAllDependentChanges(t *testing.T) {
	f := newNoteMutationFixture(t)
	mutation := f.mutation()
	mutation.Policy = noteWritePolicy(f.owner.ID, false)
	mutation.NoteUpdate.UID = new("mutation-renamed")
	mutation.NoteUpdate.CreatedTs = new(int64(1500000000))
	mutation.NoteUpdate.UpdatedTs = new(int64(1600000000))
	mutation.NoteUpdate.Pinned = new(true)
	mutation.NoteUpdate.Payload = &storepb.NotePayload{Tags: []string{"updated"}}
	mutation.Bindings = append(mutation.Bindings, &store.NoteAttachmentBinding{
		ID: f.kept.ID, UID: f.kept.UID, WasBoundToNote: true, UpdatedTs: 1600000000,
	})
	require.NoError(t, f.store.ApplyNoteMutation(f.ctx, mutation))
	got, err := f.store.GetNote(f.ctx, &store.FindNote{ID: &f.note.ID})
	require.NoError(t, err)
	require.Equal(t, "updated content", got.Content)
	require.Equal(t, "mutation-renamed", got.UID)
	require.Equal(t, int64(1500000000), got.CreatedTs)
	require.Equal(t, int64(1600000000), got.UpdatedTs)
	require.True(t, got.Pinned)
	require.Equal(t, []string{"updated"}, got.Payload.Tags)
	for _, attachment := range []*store.Attachment{f.added, f.kept} {
		got, err := f.store.GetAttachment(f.ctx, &store.FindAttachment{ID: &attachment.ID, GetBlob: true})
		require.NoError(t, err)
		require.Equal(t, &f.note.ID, got.NoteID)
		require.Equal(t, int64(1600000000), got.UpdatedTs)
		require.Equal(t, f.owner.ID, got.CreatorID)
		require.Equal(t, []byte(attachment.UID), got.Blob)
	}
	removed, err := f.store.GetAttachment(f.ctx, &store.FindAttachment{ID: &f.removed.ID})
	require.NoError(t, err)
	require.Nil(t, removed)
	relations, err := f.store.ListNoteRelations(f.ctx, &store.FindNoteRelation{NoteID: &f.note.ID})
	require.NoError(t, err)
	require.Equal(t, mutation.ReferenceRelations, relations)
}

func TestNoteMutationAttachmentOwnership(t *testing.T) {
	for _, tc := range []struct {
		name      string
		admin     bool
		thirdUser bool
		action    string
		allowed   bool
	}{
		{"author cannot bind another users upload", false, false, "bind", false},
		{"author cannot delete another users attachment", false, false, "remove", false},
		{"author can retain legacy foreign binding", false, false, "keep", true},
		{"admin can bind own upload to another users note", true, false, "bind", true},
		{"admin cannot bind unrelated users upload", true, true, "bind", false},
		{"admin can remove another users bound attachment", true, true, "remove", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			actorID := f.owner.ID
			if tc.admin {
				_, err := f.store.UpdateUser(f.ctx, &store.UpdateUser{ID: f.peer.ID, Role: new(store.RoleAdmin)})
				require.NoError(t, err)
				actorID = f.peer.ID
			}
			creatorID := f.peer.ID
			if tc.thirdUser {
				third, err := f.store.CreateUser(f.ctx, &store.User{Username: "third-user", Role: store.RoleUser})
				require.NoError(t, err)
				creatorID = third.ID
			}
			create := &store.Attachment{UID: "foreign-attachment", CreatorID: creatorID, Filename: "foreign.txt", Type: "text/plain"}
			if tc.action != "bind" {
				create.NoteID = &f.note.ID
			}
			foreign, err := f.store.CreateAttachment(f.ctx, create)
			require.NoError(t, err)
			mutation := f.mutation()
			mutation.Policy = noteWritePolicy(actorID, false)
			if tc.action == "remove" {
				mutation.RemovedAttachmentIDs = append(mutation.RemovedAttachmentIDs, foreign.ID)
			} else {
				mutation.Bindings = append(mutation.Bindings, &store.NoteAttachmentBinding{
					ID: foreign.ID, UID: foreign.UID, WasBoundToNote: tc.action == "keep", UpdatedTs: 1600000000,
				})
				mutation.RequiredAttachmentIDs = append(mutation.RequiredAttachmentIDs, foreign.ID)
			}
			before := f.state()
			err = f.store.ApplyNoteMutation(f.ctx, mutation)
			if !tc.allowed {
				require.ErrorIs(t, err, store.ErrNoteMutationConflict)
				require.Equal(t, before, f.state())
				return
			}
			require.NoError(t, err)
			got, err := f.store.GetAttachment(f.ctx, &store.FindAttachment{ID: &foreign.ID})
			require.NoError(t, err)
			if tc.action == "remove" {
				require.Nil(t, got)
			} else {
				require.Equal(t, creatorID, got.CreatorID, "binding must not transfer ownership")
				require.Equal(t, &f.note.ID, got.NoteID)
			}
		})
	}
}

func TestNoteMutationRechecksActorAndReferenceAudience(t *testing.T) {
	for _, name := range []string{"actor archived", "administrator demoted", "reference made private"} {
		t.Run(name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			mutation := f.mutation()
			mutation.Policy = noteWritePolicy(f.owner.ID, false)
			switch name {
			case "actor archived":
				_, err := f.store.UpdateUser(f.ctx, &store.UpdateUser{ID: f.owner.ID, RowStatus: new(store.Archived)})
				require.NoError(t, err)
			case "administrator demoted":
				// The instance must keep another administrator for the demotion.
				_, err := f.store.CreateUser(f.ctx, &store.User{Username: "mutation-admin", Role: store.RoleAdmin})
				require.NoError(t, err)
				_, err = f.store.UpdateUser(f.ctx, &store.UpdateUser{ID: f.peer.ID, Role: new(store.RoleAdmin)})
				require.NoError(t, err)
				mutation.Policy = noteWritePolicy(f.peer.ID, false)
				_, err = f.store.UpdateUser(f.ctx, &store.UpdateUser{ID: f.peer.ID, Role: new(store.RoleUser)})
				require.NoError(t, err)
			case "reference made private":
				target, err := f.store.CreateNote(f.ctx, &store.Note{
					UID: "foreign-reference", CreatorID: f.peer.ID, Content: "shared", Visibility: store.Public,
				})
				require.NoError(t, err)
				mutation.ReferenceRelations[0].RelatedNoteID = target.ID
				require.NoError(t, f.store.UpdateNote(f.ctx, &store.UpdateNote{ID: target.ID, Visibility: new(store.Private)}))
			default:
				t.Fatalf("unknown permission change: %s", name)
			}
			before := f.state()
			require.ErrorIs(t, f.store.ApplyNoteMutation(f.ctx, mutation), store.ErrNotePermissionDenied)
			require.Equal(t, before, f.state())
		})
	}
}

func TestNoteMutationReferenceReplacementPreservesCommentAndIncomingRelations(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "replace references"
		if !replace {
			name = "clear references"
		}
		t.Run(name, func(t *testing.T) {
			f := newNoteMutationFixture(t)
			comment, err := f.store.CreateNoteComment(f.ctx, &store.Note{
				UID: "mutation-comment", CreatorID: f.owner.ID, Content: "comment", Visibility: store.Private,
			}, f.note.ID, f.owner.ID)
			require.NoError(t, err)
			for _, relation := range []*store.NoteRelation{
				{NoteID: comment.ID, RelatedNoteID: f.target.ID, Type: store.NoteRelationReference},
				{NoteID: f.target.ID, RelatedNoteID: comment.ID, Type: store.NoteRelationReference},
			} {
				_, err := f.store.UpsertNoteRelation(f.ctx, relation)
				require.NoError(t, err)
			}
			mutation := &store.NoteMutation{
				NoteID: comment.ID, NoteCreatorID: f.owner.ID, ExpectedNoteContent: comment.Content,
				NoteUpdate: &store.UpdateNote{ID: comment.ID}, ReplaceReferenceRelations: true,
			}
			want := []*store.NoteRelation{{NoteID: comment.ID, RelatedNoteID: f.note.ID, Type: store.NoteRelationComment}}
			if replace {
				mutation.ReferenceRelations = []*store.NoteRelation{{NoteID: comment.ID, RelatedNoteID: f.next.ID, Type: store.NoteRelationReference}}
				want = append(want, mutation.ReferenceRelations...)
			}
			require.NoError(t, f.store.ApplyNoteMutation(f.ctx, mutation))
			outgoing, err := f.store.ListNoteRelations(f.ctx, &store.FindNoteRelation{NoteID: &comment.ID})
			require.NoError(t, err)
			require.ElementsMatch(t, want, outgoing)
			incoming, err := f.store.ListNoteRelations(f.ctx, &store.FindNoteRelation{RelatedNoteID: &comment.ID})
			require.NoError(t, err)
			require.Equal(t, []*store.NoteRelation{{NoteID: f.target.ID, RelatedNoteID: comment.ID, Type: store.NoteRelationReference}}, incoming)
		})
	}
}

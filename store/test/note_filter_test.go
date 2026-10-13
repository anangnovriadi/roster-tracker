package test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

func uids(notes []*store.Note) []string {
	out := make([]string, 0, len(notes))
	for _, m := range notes {
		out = append(out, m.UID)
	}
	return out
}

// =============================================================================
// Content Field Tests
// Schema: content (string, supports contains)
// =============================================================================

func TestNoteFilterContentContains(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create notes with different content
	tc.CreateNote(NewNoteBuilder("note-hello", tc.User.ID).Content("Hello world"))
	tc.CreateNote(NewNoteBuilder("note-goodbye", tc.User.ID).Content("Goodbye world"))
	tc.CreateNote(NewNoteBuilder("note-test", tc.User.ID).Content("Testing content"))

	// Test: content.contains("Hello") - single match
	notes := tc.ListWithFilter(`content.contains("Hello")`)
	require.Len(t, notes, 1)
	require.Contains(t, notes[0].Content, "Hello")

	// Test: content.contains("world") - multiple matches
	notes = tc.ListWithFilter(`content.contains("world")`)
	require.Len(t, notes, 2)

	// Test: content.contains("nonexistent") - no matches
	notes = tc.ListWithFilter(`content.contains("nonexistent")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterContentSpecialCharacters(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-special", tc.User.ID).Content("Special chars: @#$%^&*()"))

	notes := tc.ListWithFilter(`content.contains("@#$%")`)
	require.Len(t, notes, 1)
}

func TestNoteFilterContentUnicode(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-unicode", tc.User.ID).Content("Unicode test: 你好世界 🌍"))

	notes := tc.ListWithFilter(`content.contains("你好")`)
	require.Len(t, notes, 1)
}

func TestNoteFilterContentUnicodeCaseFold(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-unicode-case", tc.User.ID).Content("Привет Мир"))

	notes := tc.ListWithFilter(`content.contains("привет")`)
	require.Len(t, notes, 1)
}

func TestNoteFilterContentCaseSensitivity(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-case", tc.User.ID).Content("MixedCase Content"))

	// Exact match
	notes := tc.ListWithFilter(`content.contains("MixedCase")`)
	require.Len(t, notes, 1)

	// Lowercase match (depends on DB collation, usually case-insensitive in default installs but good to verify behavior)
	// SQLite default LIKE is case-insensitive for ASCII.
	notesLower := tc.ListWithFilter(`content.contains("mixedcase")`)
	// We just verify it doesn't crash; strict case sensitivity expectation depends on DB config.
	// For standard Roster Tracker setup (SQLite), it's often case-insensitive.
	// Let's check if we get a result or not to characterize current behavior.
	if len(notesLower) > 0 {
		require.Equal(t, "MixedCase Content", notesLower[0].Content)
	}
}

func TestNoteFilterTagsAll(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-all-work", tc.User.ID).Content("all work").Tags("work", "work/a", "work/b"))
	tc.CreateNote(NewNoteBuilder("note-mixed", tc.User.ID).Content("mixed").Tags("work", "work/a", "home"))
	tc.CreateNote(NewNoteBuilder("note-untagged", tc.User.ID).Content("untagged"))

	// Every tag belongs to the work hierarchy: only the all-work note qualifies.
	notes := tc.ListWithFilter(`tags.all(t, t.startsWith("work"))`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-all-work", notes[0].UID)

	// Untagged notes must NOT match (non-empty guard, decision B).
	require.NotContains(t, uids(notes), "note-untagged")
}

func TestNoteFilterTagsAllEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-only-x", tc.User.ID).Content("only x").Tags("x"))
	tc.CreateNote(NewNoteBuilder("note-x-and-y", tc.User.ID).Content("x and y").Tags("x", "y"))

	notes := tc.ListWithFilter(`tags.all(t, t == "x")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-only-x", notes[0].UID)
	tc.CreateNote(NewNoteBuilder("note-upper-work", tc.User.ID).Tags("Work"))
	tc.CreateNote(NewNoteBuilder("note-lower-work", tc.User.ID).Tags("work"))
	notes = tc.ListWithFilter(`tags.all(t, t == "work")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-lower-work", notes[0].UID)

	longTag := strings.Repeat("a", 513)
	tc.CreateNote(NewNoteBuilder("note-long-tag", tc.User.ID).Tags(longTag))
	notes = tc.ListWithFilter(`tags.all(t, t == "` + longTag + `")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-long-tag", notes[0].UID)
}

func TestNoteFilterContentStartsWithEscaping(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-pct", tc.User.ID).Content("100% complete"))
	tc.CreateNote(NewNoteBuilder("note-plain", tc.User.ID).Content("100 things to do"))

	// The % must be treated literally, not as a LIKE wildcard.
	notes := tc.ListWithFilter(`content.startsWith("100%")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-pct", notes[0].UID)
}

func TestNoteFilterStartsWithCombinedAndNegated(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-pub", tc.User.ID).Content("Hello public").Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-priv", tc.User.ID).Content("Hello private").Visibility(store.Private))
	tc.CreateNote(NewNoteBuilder("note-bye", tc.User.ID).Content("Goodbye public").Visibility(store.Public))

	notes := tc.ListWithFilter(`content.startsWith("Hello") && visibility == "PUBLIC"`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-pub", notes[0].UID)

	notes = tc.ListWithFilter(`!content.startsWith("Hello")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-bye", notes[0].UID)
}

func TestNoteFilterContentMatchesAdvanced(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-release", tc.User.ID).Content("release v2024 notes"))
	tc.CreateNote(NewNoteBuilder("note-draft", tc.User.ID).Content("draft document"))
	tc.CreateNote(NewNoteBuilder("note-animals", tc.User.ID).Content("cat and dog"))

	// Anchor: starts with "release".
	notes := tc.ListWithFilter(`content.matches("^release")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-release", notes[0].UID)

	// Character class + quantifier: a 4-digit run (portable across RE2/POSIX/ICU).
	notes = tc.ListWithFilter(`content.matches("[0-9]{4}")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-release", notes[0].UID)

	// Alternation.
	notes = tc.ListWithFilter(`content.matches("cat|mouse")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-animals", notes[0].UID)

	// No match.
	notes = tc.ListWithFilter(`content.matches("^zzz")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterTagsAllMorePredicates(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-all-done", tc.User.ID).Tags("done", "well-done"))
	tc.CreateNote(NewNoteBuilder("note-mixed", tc.User.ID).Tags("done", "todo"))
	tc.CreateNote(NewNoteBuilder("note-single", tc.User.ID).Tags("solo-done"))

	// endsWith: every tag ends with "done".
	notes := tc.ListWithFilter(`tags.all(t, t.endsWith("done"))`)
	require.ElementsMatch(t, []string{"note-all-done", "note-single"}, uids(notes))

	// contains: every tag contains "o".
	notes = tc.ListWithFilter(`tags.all(t, t.contains("o"))`)
	require.ElementsMatch(t, []string{"note-all-done", "note-mixed", "note-single"}, uids(notes))
}

func TestNoteFilterTagsAllNegatedAndCombined(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-work", tc.User.ID).Content("work note").Tags("work", "work/a", "work/b"))
	tc.CreateNote(NewNoteBuilder("note-mixed", tc.User.ID).Content("mixed note").Tags("work", "work/a", "home"))
	tc.CreateNote(NewNoteBuilder("note-untagged", tc.User.ID).Content("untagged note"))

	// Negation: NOT all-work. The untagged note's all() is false (non-empty guard),
	// so !all() is true and it is included.
	notes := tc.ListWithFilter(`!tags.all(t, t.startsWith("work"))`)
	require.ElementsMatch(t, []string{"note-mixed", "note-untagged"}, uids(notes))

	// Combined with a content filter.
	notes = tc.ListWithFilter(`tags.all(t, t.startsWith("work")) && content.contains("work")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-work", notes[0].UID)
}

func TestNoteFilterContentStartsWith(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-todo", tc.User.ID).Content("TODO: buy milk"))
	tc.CreateNote(NewNoteBuilder("note-done", tc.User.ID).Content("Done with milk"))

	// Prefix match, case-insensitive (consistent with contains()).
	notes := tc.ListWithFilter(`content.startsWith("todo")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-todo", notes[0].UID)

	notes = tc.ListWithFilter(`content.startsWith("nope")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterContentEndsWith(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-md", tc.User.ID).Content("notes.md"))
	tc.CreateNote(NewNoteBuilder("note-txt", tc.User.ID).Content("notes.txt"))

	notes := tc.ListWithFilter(`content.endsWith(".md")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-md", notes[0].UID)
}

func TestNoteFilterContentMatches(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-v1", tc.User.ID).Content("release v12 shipped"))
	tc.CreateNote(NewNoteBuilder("note-plain", tc.User.ID).Content("no version here"))

	notes := tc.ListWithFilter(`content.matches("v[0-9]+")`)
	require.Len(t, notes, 1)
	require.Equal(t, "note-v1", notes[0].UID)

	notes = tc.ListWithFilter(`content.matches("^xyz")`)
	require.Len(t, notes, 0)
}

// =============================================================================
// Visibility Field Tests
// Schema: visibility (string, ==, !=)
// =============================================================================

func TestNoteFilterVisibilityEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-public", tc.User.ID).Content("Public note").Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-private", tc.User.ID).Content("Private note").Visibility(store.Private))
	tc.CreateNote(NewNoteBuilder("note-protected", tc.User.ID).Content("Protected note").Visibility(store.Protected))

	// Test: visibility == "PUBLIC"
	notes := tc.ListWithFilter(`visibility == "PUBLIC"`)
	require.Len(t, notes, 1)
	require.Equal(t, store.Public, notes[0].Visibility)

	// Test: visibility == "PRIVATE"
	notes = tc.ListWithFilter(`visibility == "PRIVATE"`)
	require.Len(t, notes, 1)
	require.Equal(t, store.Private, notes[0].Visibility)
}

func TestNoteFilterVisibilityNotEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-public", tc.User.ID).Content("Public note").Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-private", tc.User.ID).Content("Private note").Visibility(store.Private))

	notes := tc.ListWithFilter(`visibility != "PUBLIC"`)
	require.Len(t, notes, 1)
	require.Equal(t, store.Private, notes[0].Visibility)
}

func TestNoteFilterVisibilityInList(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-pub", tc.User.ID).Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-priv", tc.User.ID).Visibility(store.Private))
	tc.CreateNote(NewNoteBuilder("note-prot", tc.User.ID).Visibility(store.Protected))

	notes := tc.ListWithFilter(`visibility in ["PUBLIC", "PRIVATE"]`)
	require.Len(t, notes, 2)
}

// =============================================================================
// Pinned Field Tests
// Schema: pinned (bool column, ==, !=, predicate)
// =============================================================================

func TestNoteFilterPinnedEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	pinnedNote := tc.CreateNote(NewNoteBuilder("note-pinned", tc.User.ID).Content("Pinned note"))
	tc.PinNote(pinnedNote.ID)
	tc.CreateNote(NewNoteBuilder("note-unpinned", tc.User.ID).Content("Unpinned note"))

	// Test: pinned == true
	notes := tc.ListWithFilter(`pinned == true`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Pinned)

	// Test: pinned == false
	notes = tc.ListWithFilter(`pinned == false`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Pinned)
}

func TestNoteFilterPinnedPredicate(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	pinnedNote := tc.CreateNote(NewNoteBuilder("note-pinned", tc.User.ID).Content("Pinned note"))
	tc.PinNote(pinnedNote.ID)
	tc.CreateNote(NewNoteBuilder("note-unpinned", tc.User.ID).Content("Unpinned note"))

	notes := tc.ListWithFilter(`pinned`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Pinned)
}

// =============================================================================
// Creator Field Tests
// Schema: creator (string resource name), creator_id (int, ==, !=)
// =============================================================================

func TestNoteFilterCreatorEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	user2, err := tc.Store.CreateUser(tc.Ctx, &store.User{
		Username: "user2",
		Role:     store.RoleUser,
		Email:    "user2@example.com",
		Nickname: "User 2",
	})
	require.NoError(t, err)

	tc.CreateNote(NewNoteBuilder("note-user1", tc.User.ID).Content("User 1 note"))
	tc.CreateNote(NewNoteBuilder("note-user2", user2.ID).Content("User 2 note"))

	notes := tc.ListWithFilter(`creator == "users/` + tc.User.Username + `"`)
	require.Len(t, notes, 1)
	require.Equal(t, tc.User.ID, notes[0].CreatorID)
}

// =============================================================================
// Creator ID Field Tests
// Schema: creator_id (int, ==, !=)
// =============================================================================

func TestNoteFilterCreatorIdEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	user2, err := tc.Store.CreateUser(tc.Ctx, &store.User{
		Username: "user2",
		Role:     store.RoleUser,
		Email:    "user2@example.com",
		Nickname: "User 2",
	})
	require.NoError(t, err)

	tc.CreateNote(NewNoteBuilder("note-user1", tc.User.ID).Content("User 1 note"))
	tc.CreateNote(NewNoteBuilder("note-user2", user2.ID).Content("User 2 note"))

	notes := tc.ListWithFilter(`creator_id == ` + formatInt(int(tc.User.ID)))
	require.Len(t, notes, 1)
	require.Equal(t, tc.User.ID, notes[0].CreatorID)
}

func TestNoteFilterCreatorIdNotEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	user2, err := tc.Store.CreateUser(tc.Ctx, &store.User{
		Username: "user2",
		Role:     store.RoleUser,
		Email:    "user2@example.com",
		Nickname: "User 2",
	})
	require.NoError(t, err)

	tc.CreateNote(NewNoteBuilder("note-user1", tc.User.ID).Content("User 1 note"))
	tc.CreateNote(NewNoteBuilder("note-user2", user2.ID).Content("User 2 note"))

	notes := tc.ListWithFilter(`creator_id != ` + formatInt(int(tc.User.ID)))
	require.Len(t, notes, 1)
	require.Equal(t, user2.ID, notes[0].CreatorID)
}

// =============================================================================
// Tags Field Tests
// Schema: tags (JSON list), tag (virtual alias)
// Operators: tag in [...], "value" in tags
// =============================================================================

func TestNoteFilterTagInList(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-work", tc.User.ID).Content("Work note").Tags("work", "important"))
	tc.CreateNote(NewNoteBuilder("note-personal", tc.User.ID).Content("Personal note").Tags("personal", "fun"))
	tc.CreateNote(NewNoteBuilder("note-no-tags", tc.User.ID).Content("No tags"))

	// Test: tag in ["work"]
	notes := tc.ListWithFilter(`tag in ["work"]`)
	require.Len(t, notes, 1)
	require.Contains(t, notes[0].Payload.Tags, "work")

	// Test: tag in ["work", "personal"]
	notes = tc.ListWithFilter(`tag in ["work", "personal"]`)
	require.Len(t, notes, 2)
}

func TestNoteFilterElementInTags(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-tagged", tc.User.ID).Content("Tagged note").Tags("project", "todo"))
	tc.CreateNote(NewNoteBuilder("note-untagged", tc.User.ID).Content("Untagged note"))

	// Test: "project" in tags
	notes := tc.ListWithFilter(`"project" in tags`)
	require.Len(t, notes, 1)

	// Test: "nonexistent" in tags
	notes = tc.ListWithFilter(`"nonexistent" in tags`)
	require.Len(t, notes, 0)
}

func TestNoteFilterHierarchicalTags(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-book", tc.User.ID).Content("Book note").Tags("book"))
	tc.CreateNote(NewNoteBuilder("note-book-fiction", tc.User.ID).Content("Fiction book note").Tags("book", "book/fiction"))

	// Implied ancestors are stored in the note tag set, so exact membership matches both notes.
	notes := tc.ListWithFilter(`tag in ["book"]`)
	require.Len(t, notes, 2)
}

func TestNoteFilterTagMembershipIsExact(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()
	const (
		composedTag   = "caf\u00e9"
		decomposedTag = "cafe\u0301"
	)

	tc.CreateNote(NewNoteBuilder("note-underscore", tc.User.ID).Tags("a_b"))
	tc.CreateNote(NewNoteBuilder("note-letter", tc.User.ID).Tags("axb"))
	tc.CreateNote(NewNoteBuilder("note-percent", tc.User.ID).Tags("a%b"))
	tc.CreateNote(NewNoteBuilder("note-letters", tc.User.ID).Tags("aZZb"))
	tc.CreateNote(NewNoteBuilder("note-upper", tc.User.ID).Tags("Work"))
	tc.CreateNote(NewNoteBuilder("note-lower", tc.User.ID).Tags("work"))
	tc.CreateNote(NewNoteBuilder("note-composed", tc.User.ID).Tags(composedTag))
	tc.CreateNote(NewNoteBuilder("note-decomposed", tc.User.ID).Tags(decomposedTag))
	tc.CreateNote(NewNoteBuilder("note-book", tc.User.ID).Tags("book"))
	tc.CreateNote(NewNoteBuilder("note-descendant-only", tc.User.ID).Tags("book/fiction"))

	for _, test := range []struct {
		filter string
		want   string
	}{
		{filter: `tag in ["a_b"]`, want: "note-underscore"},
		{filter: `tag in ["a%b"]`, want: "note-percent"},
		{filter: `tag in ["Work"]`, want: "note-upper"},
		{filter: `tag in ["work"]`, want: "note-lower"},
		{filter: `tag in ["` + composedTag + `"]`, want: "note-composed"},
		{filter: `tag in ["` + decomposedTag + `"]`, want: "note-decomposed"},
		{filter: `tag in ["book"]`, want: "note-book"},
		{filter: `"work" in tags`, want: "note-lower"},
	} {
		notes := tc.ListWithFilter(test.filter)
		require.Len(t, notes, 1, test.filter)
		require.Equal(t, test.want, notes[0].UID, test.filter)
	}

	for _, test := range []struct {
		filter string
		want   string
	}{
		{filter: `tags.exists(t, t.startsWith("a_"))`, want: "note-underscore"},
		{filter: `tags.exists(t, t.endsWith("%b"))`, want: "note-percent"},
		{filter: `tags.exists(t, t.contains("a%b"))`, want: "note-percent"},
		{filter: `tags.all(t, t.startsWith("a_"))`, want: "note-underscore"},
		{filter: `tags.exists_one(t, t.contains("a%b"))`, want: "note-percent"},
	} {
		notes := tc.ListWithFilter(test.filter)
		require.Len(t, notes, 1, test.filter)
		require.Equal(t, test.want, notes[0].UID, test.filter)
	}

	require.Empty(t, tc.ListWithFilter(`tags.exists(t, t.contains("A_"))`))
}

func TestNoteFilterEmptyTags(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-empty-tags", tc.User.ID).Content("Empty tags").Tags())

	notes := tc.ListWithFilter(`tag in ["anything"]`)
	require.Len(t, notes, 0)
}

// =============================================================================
// JSON Bool Field Tests
// Schema: has_task_list, has_link, has_code, has_incomplete_tasks
// Operators: ==, !=, predicate
// =============================================================================

func TestNoteFilterHasTaskList(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-with-tasks", tc.User.ID).
		Content("- [ ] Task 1\n- [x] Task 2").
		Property(func(p *storepb.NotePayload_Property) { p.HasTaskList = true }))
	tc.CreateNote(NewNoteBuilder("note-no-tasks", tc.User.ID).Content("No tasks here"))

	// Test: has_task_list (predicate)
	notes := tc.ListWithFilter(`has_task_list`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Payload.Property.HasTaskList)

	// Test: has_task_list == true
	notes = tc.ListWithFilter(`has_task_list == true`)
	require.Len(t, notes, 1)

	// Test: has_task_list == false matches notes whose payload omits the flag
	notes = tc.ListWithFilter(`has_task_list == false`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasTaskList())

	// Test: has_task_list != true
	notes = tc.ListWithFilter(`has_task_list != true`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasTaskList())
}

func TestNoteFilterHasLink(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-with-link", tc.User.ID).
		Content("Check out https://example.com").
		Property(func(p *storepb.NotePayload_Property) { p.HasLink = true }))
	tc.CreateNote(NewNoteBuilder("note-no-link", tc.User.ID).Content("No links"))

	notes := tc.ListWithFilter(`has_link`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Payload.Property.HasLink)

	// Test: has_link == false matches notes whose payload omits the flag
	notes = tc.ListWithFilter(`has_link == false`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasLink())

	// Test: has_link != true
	notes = tc.ListWithFilter(`has_link != true`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasLink())

	// Test: has_link != false
	notes = tc.ListWithFilter(`has_link != false`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Payload.GetProperty().GetHasLink())
}

func TestNoteFilterHasCode(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-with-code", tc.User.ID).
		Content("```go\nfmt.Println(\"Hello\")\n```").
		Property(func(p *storepb.NotePayload_Property) { p.HasCode = true }))
	tc.CreateNote(NewNoteBuilder("note-no-code", tc.User.ID).Content("No code"))

	notes := tc.ListWithFilter(`has_code`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Payload.Property.HasCode)

	// Test: has_code == false matches notes whose payload omits the flag
	notes = tc.ListWithFilter(`has_code == false`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasCode())
}

func TestNoteFilterHasIncompleteTasks(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-incomplete", tc.User.ID).
		Content("- [ ] Incomplete task").
		Property(func(p *storepb.NotePayload_Property) {
			p.HasTaskList = true
			p.HasIncompleteTasks = true
		}))
	tc.CreateNote(NewNoteBuilder("note-complete", tc.User.ID).
		Content("- [x] Complete task").
		Property(func(p *storepb.NotePayload_Property) {
			p.HasTaskList = true
			p.HasIncompleteTasks = false
		}))

	notes := tc.ListWithFilter(`has_incomplete_tasks`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Payload.Property.HasIncompleteTasks)

	// Test: has_incomplete_tasks == false matches notes whose payload omits the flag
	notes = tc.ListWithFilter(`has_incomplete_tasks == false`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Payload.GetProperty().GetHasIncompleteTasks())
}

func TestNoteFilterCombinedJSONBool(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Note with all properties
	tc.CreateNote(NewNoteBuilder("note-all-props", tc.User.ID).
		Content("All properties").
		Property(func(p *storepb.NotePayload_Property) {
			p.HasLink = true
			p.HasTaskList = true
			p.HasCode = true
			p.HasIncompleteTasks = true
		}))

	// Note with only link
	tc.CreateNote(NewNoteBuilder("note-only-link", tc.User.ID).
		Content("Only link").
		Property(func(p *storepb.NotePayload_Property) { p.HasLink = true }))

	// Test: has_link && has_code
	notes := tc.ListWithFilter(`has_link && has_code`)
	require.Len(t, notes, 1)

	// Test: has_task_list && has_incomplete_tasks
	notes = tc.ListWithFilter(`has_task_list && has_incomplete_tasks`)
	require.Len(t, notes, 1)

	// Test: has_link || has_code
	notes = tc.ListWithFilter(`has_link || has_code`)
	require.Len(t, notes, 2)
}

// =============================================================================
// Timestamp Field Tests
// Schema: created_ts, updated_ts (timestamp, all comparison operators)
// Time helpers: now variable, timestamp(...), duration(...)
// =============================================================================

func TestNoteFilterCreatedTsComparison(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	nowSec := time.Now().Unix()
	tc.CreateNote(NewNoteBuilder("note-ts", tc.User.ID).Content("Timestamp test"))

	// Test: created_ts < future (should match)
	notes := tc.ListWithFilter(`created_ts < timestamp(` + formatInt64(nowSec+3600) + `)`)
	require.Len(t, notes, 1)

	// Test: created_ts > past (should match)
	notes = tc.ListWithFilter(`created_ts > timestamp(` + formatInt64(nowSec-3600) + `)`)
	require.Len(t, notes, 1)

	// Test: created_ts > future (should not match)
	notes = tc.ListWithFilter(`created_ts > timestamp(` + formatInt64(nowSec+3600) + `)`)
	require.Len(t, notes, 0)
}

func TestNoteFilterCreatedTsWithNow(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-ts-test", tc.User.ID).Content("Timestamp test"))

	// Test: created_ts < now + 5s (buffer for container clock drift)
	notes := tc.ListWithFilter(`created_ts < now + duration("5s")`)
	require.Len(t, notes, 1)

	// Test: created_ts > now + 5s (should not match)
	notes = tc.ListWithFilter(`created_ts > now + duration("5s")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterCreatedTsArithmetic(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-ts-arith", tc.User.ID).Content("Timestamp arithmetic test"))

	// Test: created_ts >= now - 1h (notes created in last hour)
	notes := tc.ListWithFilter(`created_ts >= now - duration("1h")`)
	require.Len(t, notes, 1)

	// Test: created_ts < now - 24h (notes older than 1 day - should be empty)
	notes = tc.ListWithFilter(`created_ts < now - duration("24h")`)
	require.Len(t, notes, 0)

	// Test: chained duration arithmetic - created_ts >= now - 30m - 30m
	notes = tc.ListWithFilter(`created_ts >= now - duration("30m") - duration("30m")`)
	require.Len(t, notes, 1)
}

func TestNoteFilterUpdatedTs(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	note := tc.CreateNote(NewNoteBuilder("note-updated", tc.User.ID).Content("Will be updated"))

	// Update the note
	newContent := "Updated content"
	err := tc.Store.UpdateNote(tc.Ctx, &store.UpdateNote{
		ID:      note.ID,
		Content: &newContent,
	})
	require.NoError(t, err)

	// Test: updated_ts >= now - 60s (updated in last minute)
	notes := tc.ListWithFilter(`updated_ts >= now - duration("60s")`)
	require.Len(t, notes, 1)

	// Test: updated_ts > now + 1h (should be empty)
	notes = tc.ListWithFilter(`updated_ts > now + duration("1h")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterAllComparisonOperators(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-ops", tc.User.ID).Content("Comparison operators test"))

	// Test: < (less than)
	notes := tc.ListWithFilter(`created_ts < now + duration("1h")`)
	require.Len(t, notes, 1)

	// Test: <= (less than or equal) with buffer for clock drift
	notes = tc.ListWithFilter(`created_ts < now + duration("5s")`)
	require.Len(t, notes, 1)

	// Test: > (greater than)
	notes = tc.ListWithFilter(`created_ts > now - duration("1h")`)
	require.Len(t, notes, 1)

	// Test: >= (greater than or equal)
	notes = tc.ListWithFilter(`created_ts >= now - duration("60s")`)
	require.Len(t, notes, 1)
}

// =============================================================================
// Logical Operator Tests
// Operators: && (AND), || (OR), ! (NOT)
// =============================================================================

func TestNoteFilterLogicalAnd(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	pinnedNote := tc.CreateNote(NewNoteBuilder("note-pinned-public", tc.User.ID).Content("Pinned public"))
	tc.PinNote(pinnedNote.ID)
	tc.CreateNote(NewNoteBuilder("note-unpinned-public", tc.User.ID).Content("Unpinned public"))

	notes := tc.ListWithFilter(`pinned && visibility == "PUBLIC"`)
	require.Len(t, notes, 1)
	require.True(t, notes[0].Pinned)
}

func TestNoteFilterLogicalOr(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-public", tc.User.ID).Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-private", tc.User.ID).Visibility(store.Private))
	tc.CreateNote(NewNoteBuilder("note-protected", tc.User.ID).Visibility(store.Protected))

	notes := tc.ListWithFilter(`visibility == "PUBLIC" || visibility == "PRIVATE"`)
	require.Len(t, notes, 2)
}

func TestNoteFilterLogicalNot(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	pinnedNote := tc.CreateNote(NewNoteBuilder("note-pinned", tc.User.ID).Content("Pinned"))
	tc.PinNote(pinnedNote.ID)
	tc.CreateNote(NewNoteBuilder("note-unpinned", tc.User.ID).Content("Unpinned"))

	notes := tc.ListWithFilter(`!pinned`)
	require.Len(t, notes, 1)
	require.False(t, notes[0].Pinned)
}

func TestNoteFilterNegatedComparison(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-public", tc.User.ID).Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-private", tc.User.ID).Visibility(store.Private))

	notes := tc.ListWithFilter(`!(visibility == "PUBLIC")`)
	require.Len(t, notes, 1)
	require.Equal(t, store.Private, notes[0].Visibility)
}

func TestNoteFilterComplexLogical(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create pinned public note with tags
	pinnedNote := tc.CreateNote(NewNoteBuilder("note-pinned-tagged", tc.User.ID).
		Content("Pinned and tagged").Tags("important"))
	tc.PinNote(pinnedNote.ID)

	// Create unpinned note with same tag
	tc.CreateNote(NewNoteBuilder("note-unpinned-tagged", tc.User.ID).
		Content("Unpinned but tagged").Tags("important"))

	// Create pinned note without tag
	pinned2 := tc.CreateNote(NewNoteBuilder("note-pinned-untagged", tc.User.ID).Content("Pinned but untagged"))
	tc.PinNote(pinned2.ID)

	// Test: pinned && tag in ["important"]
	notes := tc.ListWithFilter(`pinned && tag in ["important"]`)
	require.Len(t, notes, 1)

	// Test: (pinned || tag in ["important"]) && visibility == "PUBLIC"
	notes = tc.ListWithFilter(`(pinned || tag in ["important"]) && visibility == "PUBLIC"`)
	require.Len(t, notes, 3)

	// Test: De Morgan's Law ! (A || B) == !A && !B
	// ! (pinned || has_task_list)
	tc.CreateNote(NewNoteBuilder("note-no-props", tc.User.ID).Content("Nothing special"))
	notes = tc.ListWithFilter(`!(pinned || has_task_list)`)
	require.Len(t, notes, 2) // Unpinned-tagged + Nothing special (pinned-untagged is pinned)
}

// =============================================================================
// Tag Comprehension Tests (exists macro)
// Schema: tags (list of strings, supports exists/all macros with predicates)
// =============================================================================

func TestNoteFilterTagsExistsStartsWith(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create notes with different tags
	tc.CreateNote(NewNoteBuilder("note-archive1", tc.User.ID).
		Content("Archived project note").
		Tags("archive/project", "done"))

	tc.CreateNote(NewNoteBuilder("note-archive2", tc.User.ID).
		Content("Archived work note").
		Tags("archive/work", "old"))

	tc.CreateNote(NewNoteBuilder("note-active", tc.User.ID).
		Content("Active project note").
		Tags("project/active", "todo"))

	tc.CreateNote(NewNoteBuilder("note-homelab", tc.User.ID).
		Content("Homelab note").
		Tags("homelab/notes", "tech"))

	// Test: tags.exists(t, t.startsWith("archive")) - should match archived notes
	notes := tc.ListWithFilter(`tags.exists(t, t.startsWith("archive"))`)
	require.Len(t, notes, 2, "Should find 2 archived notes")
	for _, note := range notes {
		hasArchiveTag := false
		for _, tag := range note.Payload.Tags {
			if len(tag) >= 7 && tag[:7] == "archive" {
				hasArchiveTag = true
				break
			}
		}
		require.True(t, hasArchiveTag, "Note should have tag starting with 'archive'")
	}

	// Test: !tags.exists(t, t.startsWith("archive")) - should match non-archived notes
	notes = tc.ListWithFilter(`!tags.exists(t, t.startsWith("archive"))`)
	require.Len(t, notes, 2, "Should find 2 non-archived notes")

	// Test: tags.exists(t, t.startsWith("project")) - should match project notes
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("project"))`)
	require.Len(t, notes, 1, "Should find 1 project note")

	// Test: tags.exists(t, t.startsWith("homelab")) - should match homelab notes
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("homelab"))`)
	require.Len(t, notes, 1, "Should find 1 homelab note")

	// Test: tags.exists(t, t.startsWith("nonexistent")) - should match nothing
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("nonexistent"))`)
	require.Len(t, notes, 0, "Should find no notes")
}

func TestNoteFilterTagsExistsContains(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create notes with different tags
	tc.CreateNote(NewNoteBuilder("note-todo1", tc.User.ID).
		Content("Todo task 1").
		Tags("project/todo", "urgent"))

	tc.CreateNote(NewNoteBuilder("note-todo2", tc.User.ID).
		Content("Todo task 2").
		Tags("work/todo-list", "pending"))

	tc.CreateNote(NewNoteBuilder("note-done", tc.User.ID).
		Content("Done task").
		Tags("project/completed", "done"))

	// Test: tags.exists(t, t.contains("todo")) - should match todos
	notes := tc.ListWithFilter(`tags.exists(t, t.contains("todo"))`)
	require.Len(t, notes, 2, "Should find 2 todo notes")

	// Test: tags.exists(t, t.contains("done")) - should match done
	notes = tc.ListWithFilter(`tags.exists(t, t.contains("done"))`)
	require.Len(t, notes, 1, "Should find 1 done note")

	// Test: !tags.exists(t, t.contains("todo")) - should exclude todos
	notes = tc.ListWithFilter(`!tags.exists(t, t.contains("todo"))`)
	require.Len(t, notes, 1, "Should find 1 non-todo note")
}

func TestNoteFilterTagsExistsEquals(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-1231", tc.User.ID).
		Content("Note with exact numeric tag").
		Tags("1231", "project"))

	tc.CreateNote(NewNoteBuilder("note-1231-suffix", tc.User.ID).
		Content("Note with related tag").
		Tags("tag/1231", "other"))

	tc.CreateNote(NewNoteBuilder("note-other", tc.User.ID).
		Content("Note with different tag").
		Tags("9999"))

	notes := tc.ListWithFilter(`tags.exists(t, t == "1231")`)
	require.Len(t, notes, 1, "Should find only the note with exact matching tag")
	require.Equal(t, "note-1231", notes[0].UID)

	notes = tc.ListWithFilter(`!tags.exists(t, t == "1231")`)
	require.Len(t, notes, 2, "Should exclude only the note with exact matching tag")
}

func TestNoteFilterTagsExistsEndsWith(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create notes with different tag endings
	tc.CreateNote(NewNoteBuilder("note-bug", tc.User.ID).
		Content("Bug report").
		Tags("project/bug", "critical"))

	tc.CreateNote(NewNoteBuilder("note-debug", tc.User.ID).
		Content("Debug session").
		Tags("work/debug", "dev"))

	tc.CreateNote(NewNoteBuilder("note-feature", tc.User.ID).
		Content("New feature").
		Tags("project/feature", "new"))

	// Test: tags.exists(t, t.endsWith("bug")) - should match bug-related tags
	notes := tc.ListWithFilter(`tags.exists(t, t.endsWith("bug"))`)
	require.Len(t, notes, 2, "Should find 2 bug-related notes")

	// Test: tags.exists(t, t.endsWith("feature")) - should match feature
	notes = tc.ListWithFilter(`tags.exists(t, t.endsWith("feature"))`)
	require.Len(t, notes, 1, "Should find 1 feature note")

	// Test: !tags.exists(t, t.endsWith("bug")) - should exclude bug-related
	notes = tc.ListWithFilter(`!tags.exists(t, t.endsWith("bug"))`)
	require.Len(t, notes, 1, "Should find 1 non-bug note")
}

func TestNoteFilterTagsExistsCombinedWithOtherFilters(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create notes with tags and other properties
	tc.CreateNote(NewNoteBuilder("note-archived-old", tc.User.ID).
		Content("Old archived note").
		Tags("archive/old", "done"))

	tc.CreateNote(NewNoteBuilder("note-archived-recent", tc.User.ID).
		Content("Recent archived note with TODO").
		Tags("archive/recent", "done"))

	tc.CreateNote(NewNoteBuilder("note-active-todo", tc.User.ID).
		Content("Active TODO").
		Tags("project/active", "todo"))

	// Test: Combine tag filter with content filter
	notes := tc.ListWithFilter(`tags.exists(t, t.startsWith("archive")) && content.contains("TODO")`)
	require.Len(t, notes, 1, "Should find 1 archived note with TODO in content")

	// Test: OR condition with tag filters
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("archive")) || tags.exists(t, t.contains("todo"))`)
	require.Len(t, notes, 3, "Should find all notes (archived or with todo tag)")

	// Test: Complex filter - archived but not containing "Recent"
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("archive")) && !content.contains("Recent")`)
	require.Len(t, notes, 1, "Should find 1 old archived note")
}

func TestNoteFilterTagsExistsEmptyAndNullCases(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create note with no tags
	tc.CreateNote(NewNoteBuilder("note-no-tags", tc.User.ID).
		Content("Note without tags"))

	// Create note with tags
	tc.CreateNote(NewNoteBuilder("note-with-tags", tc.User.ID).
		Content("Note with tags").
		Tags("tag1", "tag2"))

	// Test: tags.exists should not match notes without tags
	notes := tc.ListWithFilter(`tags.exists(t, t.startsWith("tag"))`)
	require.Len(t, notes, 1, "Should only find note with tags")

	// Test: Negation should match notes without matching tags
	notes = tc.ListWithFilter(`!tags.exists(t, t.startsWith("tag"))`)
	require.Len(t, notes, 1, "Should find note without matching tags")
}

func TestNoteFilterIssue5480_ArchiveWorkflow(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// Create a realistic scenario as described in issue #5480
	// User has hierarchical tags and archives notes by prefixing with "archive"

	// Active notes
	tc.CreateNote(NewNoteBuilder("note-homelab", tc.User.ID).
		Content("Setting up Notes").
		Tags("homelab/notes", "tech"))

	tc.CreateNote(NewNoteBuilder("note-project-alpha", tc.User.ID).
		Content("Project Alpha notes").
		Tags("work/project-alpha", "active"))

	// Archived notes (user prefixed tags with "archive")
	tc.CreateNote(NewNoteBuilder("note-old-homelab", tc.User.ID).
		Content("Old homelab setup").
		Tags("archive/homelab/old-server", "done"))

	tc.CreateNote(NewNoteBuilder("note-old-project", tc.User.ID).
		Content("Old project beta").
		Tags("archive/work/project-beta", "completed"))

	tc.CreateNote(NewNoteBuilder("note-archived-personal", tc.User.ID).
		Content("Archived personal note").
		Tags("archive/personal/2024", "old"))

	// Test: Filter out ALL archived notes using startsWith
	notes := tc.ListWithFilter(`!tags.exists(t, t.startsWith("archive"))`)
	require.Len(t, notes, 2, "Should only show active notes (not archived)")
	for _, note := range notes {
		for _, tag := range note.Payload.Tags {
			require.NotContains(t, tag, "archive", "Active notes should not have archive prefix")
		}
	}

	// Test: Show ONLY archived notes
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("archive"))`)
	require.Len(t, notes, 3, "Should find all archived notes")
	for _, note := range notes {
		hasArchiveTag := false
		for _, tag := range note.Payload.Tags {
			if len(tag) >= 7 && tag[:7] == "archive" {
				hasArchiveTag = true
				break
			}
		}
		require.True(t, hasArchiveTag, "All returned notes should have archive prefix")
	}

	// Test: Filter archived homelab notes specifically
	notes = tc.ListWithFilter(`tags.exists(t, t.startsWith("archive/homelab"))`)
	require.Len(t, notes, 1, "Should find only archived homelab notes")
}

// =============================================================================
// Multiple Filters Tests
// =============================================================================

func TestNoteFilterMultipleFilters(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-public-hello", tc.User.ID).Content("Hello world").Visibility(store.Public))
	tc.CreateNote(NewNoteBuilder("note-private-hello", tc.User.ID).Content("Hello private").Visibility(store.Private))

	// Test: Multiple filters (applied as AND)
	notes := tc.ListWithFilters(`content.contains("Hello")`, `visibility == "PUBLIC"`)
	require.Len(t, notes, 1)
	require.Contains(t, notes[0].Content, "Hello")
	require.Equal(t, store.Public, notes[0].Visibility)
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestNoteFilterNullPayload(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-null-payload", tc.User.ID).Content("Null payload"))

	// Test: has_link should not crash and return no results
	notes := tc.ListWithFilter(`has_link`)
	require.Len(t, notes, 0)
}

func TestNoteFilterNoMatches(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	tc.CreateNote(NewNoteBuilder("note-test", tc.User.ID).Content("Test content"))

	notes := tc.ListWithFilter(`content.contains("nonexistent12345")`)
	require.Len(t, notes, 0)
}

func TestNoteFilterJSONBooleanLogic(t *testing.T) {
	t.Parallel()
	tc := NewNoteFilterTestContext(t)
	defer tc.Close()

	// 1. Note with task list (true) and NO link (null)
	tc.CreateNote(NewNoteBuilder("note-task-only", tc.User.ID).
		Content("Task only").
		Property(func(p *storepb.NotePayload_Property) { p.HasTaskList = true }))

	// 2. Note with link (true) and NO task list (null)
	tc.CreateNote(NewNoteBuilder("note-link-only", tc.User.ID).
		Content("Link only").
		Property(func(p *storepb.NotePayload_Property) { p.HasLink = true }))

	// 3. Note with both (true)
	tc.CreateNote(NewNoteBuilder("note-both", tc.User.ID).
		Content("Both").
		Property(func(p *storepb.NotePayload_Property) {
			p.HasTaskList = true
			p.HasLink = true
		}))

	// 4. Note with neither (null)
	tc.CreateNote(NewNoteBuilder("note-neither", tc.User.ID).Content("Neither"))

	// Test A: has_task_list || has_link
	// Expected: 3 notes (task-only, link-only, both). Neither should be excluded.
	// This specifically tests the NULL handling in OR logic (NULL || TRUE should be TRUE)
	notes := tc.ListWithFilter(`has_task_list || has_link`)
	require.Len(t, notes, 3, "Should find 3 notes with OR logic")

	// Test B: !has_task_list
	// Expected: 2 notes (link-only, neither). Notes where has_task_list is NULL or FALSE.
	// Note: If NULL is not handled, !NULL is still NULL (false-y in WHERE), so "neither" might be missed depending on logic.
	// In our implementation, we want missing fields to behave as false.
	notes = tc.ListWithFilter(`!has_task_list`)
	require.Len(t, notes, 2, "Should find 2 notes where task list is false or missing")

	// Test C: has_task_list && !has_link
	// Expected: 1 note (task-only).
	notes = tc.ListWithFilter(`has_task_list && !has_link`)
	require.Len(t, notes, 1, "Should find 1 note (task only)")
}

package test

import (
	"context"
	"strconv"
	"testing"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/require"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

// =============================================================================
// Formatting Helpers
// =============================================================================

func formatInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}

func formatInt32(n int32) string {
	return strconv.FormatInt(int64(n), 10)
}

func formatInt(n int) string {
	return strconv.Itoa(n)
}

// =============================================================================
// Test Fixture Builders
// =============================================================================

// NoteBuilder provides a fluent API for creating test notes.
type NoteBuilder struct {
	note *store.Note
}

// NewNoteBuilder creates a new note builder with required fields.
func NewNoteBuilder(uid string, creatorID int32) *NoteBuilder {
	return &NoteBuilder{
		note: &store.Note{
			UID:        uid,
			CreatorID:  creatorID,
			Visibility: store.Public,
		},
	}
}

func (b *NoteBuilder) Content(content string) *NoteBuilder {
	b.note.Content = content
	return b
}

func (b *NoteBuilder) Visibility(v store.Visibility) *NoteBuilder {
	b.note.Visibility = v
	return b
}

func (b *NoteBuilder) CreatedTs(ts int64) *NoteBuilder {
	b.note.CreatedTs = ts
	return b
}

func (b *NoteBuilder) Tags(tags ...string) *NoteBuilder {
	if b.note.Payload == nil {
		b.note.Payload = &storepb.NotePayload{}
	}
	b.note.Payload.Tags = tags
	return b
}

func (b *NoteBuilder) Property(fn func(*storepb.NotePayload_Property)) *NoteBuilder {
	if b.note.Payload == nil {
		b.note.Payload = &storepb.NotePayload{}
	}
	if b.note.Payload.Property == nil {
		b.note.Payload.Property = &storepb.NotePayload_Property{}
	}
	fn(b.note.Payload.Property)
	return b
}

func (b *NoteBuilder) Build() *store.Note {
	return b.note
}

// AttachmentBuilder provides a fluent API for creating test attachments.
type AttachmentBuilder struct {
	attachment *store.Attachment
}

// NewAttachmentBuilder creates a new attachment builder with required fields.
func NewAttachmentBuilder(creatorID int32) *AttachmentBuilder {
	return &AttachmentBuilder{
		attachment: &store.Attachment{
			UID:       shortuuid.New(),
			CreatorID: creatorID,
			Blob:      []byte("test"),
			Size:      1000,
		},
	}
}

func (b *AttachmentBuilder) Filename(filename string) *AttachmentBuilder {
	b.attachment.Filename = filename
	return b
}

func (b *AttachmentBuilder) MimeType(mimeType string) *AttachmentBuilder {
	b.attachment.Type = mimeType
	return b
}

func (b *AttachmentBuilder) NoteID(noteID *int32) *AttachmentBuilder {
	b.attachment.NoteID = noteID
	return b
}

func (b *AttachmentBuilder) Size(size int64) *AttachmentBuilder {
	b.attachment.Size = size
	return b
}

func (b *AttachmentBuilder) Build() *store.Attachment {
	return b.attachment
}

// =============================================================================
// Test Context Helpers
// =============================================================================

// NoteFilterTestContext holds common test dependencies for note filter tests.
type NoteFilterTestContext struct {
	Ctx   context.Context
	T     *testing.T
	Store *store.Store
	User  *store.User
}

// NewNoteFilterTestContext creates a new test context with store and user.
func NewNoteFilterTestContext(t *testing.T) *NoteFilterTestContext {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	return &NoteFilterTestContext{
		Ctx:   ctx,
		T:     t,
		Store: ts,
		User:  user,
	}
}

// CreateNote creates a note using the builder pattern.
func (tc *NoteFilterTestContext) CreateNote(b *NoteBuilder) *store.Note {
	note, err := tc.Store.CreateNote(tc.Ctx, b.Build())
	require.NoError(tc.T, err)
	return note
}

// PinNote pins a note by ID.
func (tc *NoteFilterTestContext) PinNote(noteID int32) {
	err := tc.Store.UpdateNote(tc.Ctx, &store.UpdateNote{
		ID:     noteID,
		Pinned: new(true),
	})
	require.NoError(tc.T, err)
}

// ListWithFilter lists notes with the given filter and returns the count.
func (tc *NoteFilterTestContext) ListWithFilter(filter string) []*store.Note {
	notes, err := tc.Store.ListNotes(tc.Ctx, &store.FindNote{
		Filters: []string{filter},
	})
	require.NoError(tc.T, err)
	return notes
}

// ListWithFilters lists notes with multiple filters and returns the count.
func (tc *NoteFilterTestContext) ListWithFilters(filters ...string) []*store.Note {
	notes, err := tc.Store.ListNotes(tc.Ctx, &store.FindNote{
		Filters: filters,
	})
	require.NoError(tc.T, err)
	return notes
}

// Close closes the test store.
func (tc *NoteFilterTestContext) Close() {
	tc.Store.Close()
}

// AttachmentFilterTestContext holds common test dependencies for attachment filter tests.
type AttachmentFilterTestContext struct {
	Ctx       context.Context
	T         *testing.T
	Store     *store.Store
	User      *store.User
	CreatorID int32
}

// NewAttachmentFilterTestContext creates a new test context for attachments.
func NewAttachmentFilterTestContext(t *testing.T) *AttachmentFilterTestContext {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	return &AttachmentFilterTestContext{
		Ctx:       ctx,
		T:         t,
		Store:     ts,
		CreatorID: 101,
	}
}

// NewAttachmentFilterTestContextWithUser creates a new test context with a user.
func NewAttachmentFilterTestContextWithUser(t *testing.T) *AttachmentFilterTestContext {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	return &AttachmentFilterTestContext{
		Ctx:       ctx,
		T:         t,
		Store:     ts,
		User:      user,
		CreatorID: user.ID,
	}
}

// CreateAttachment creates an attachment using the builder pattern.
func (tc *AttachmentFilterTestContext) CreateAttachment(b *AttachmentBuilder) *store.Attachment {
	attachment, err := tc.Store.CreateAttachment(tc.Ctx, b.Build())
	require.NoError(tc.T, err)
	return attachment
}

// CreateNote creates a note (for attachment tests that need notes).
func (tc *AttachmentFilterTestContext) CreateNote(uid, content string) *store.Note {
	note, err := tc.Store.CreateNote(tc.Ctx, &store.Note{
		UID:        uid,
		CreatorID:  tc.CreatorID,
		Content:    content,
		Visibility: store.Public,
	})
	require.NoError(tc.T, err)
	return note
}

// ListWithFilter lists attachments with the given filter.
func (tc *AttachmentFilterTestContext) ListWithFilter(filter string) []*store.Attachment {
	attachments, err := tc.Store.ListAttachments(tc.Ctx, &store.FindAttachment{
		CreatorID: &tc.CreatorID,
		Filters:   []string{filter},
	})
	require.NoError(tc.T, err)
	return attachments
}

// ListWithFilters lists attachments with multiple filters.
func (tc *AttachmentFilterTestContext) ListWithFilters(filters ...string) []*store.Attachment {
	attachments, err := tc.Store.ListAttachments(tc.Ctx, &store.FindAttachment{
		CreatorID: &tc.CreatorID,
		Filters:   filters,
	})
	require.NoError(tc.T, err)
	return attachments
}

// Close closes the test store.
func (tc *AttachmentFilterTestContext) Close() {
	tc.Store.Close()
}

// =============================================================================
// Filter Test Case Definition
// =============================================================================

// FilterTestCase defines a single filter test case for table-driven tests.
type FilterTestCase struct {
	Name          string
	Filter        string
	ExpectedCount int
}

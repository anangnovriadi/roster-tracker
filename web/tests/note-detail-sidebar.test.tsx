import { create } from "@bufbuild/protobuf";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NoteDetailSidebar from "@/components/NoteDetailSidebar/NoteDetailSidebar";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { State } from "@/types/proto/api/common_pb";
import {
  LocationSchema,
  NoteRelation_NoteRefSchema,
  NoteRelation_Type,
  NoteRelationSchema,
  NoteSchema,
  Visibility,
} from "@/types/proto/api/note_service_pb";

const copyToClipboard = vi.hoisted(() => vi.fn());
const currentUserState = vi.hoisted(() => ({ value: { name: "users/alice" } as { name: string } | undefined }));

vi.mock("copy-to-clipboard", () => ({ default: copyToClipboard }));
vi.mock("react-hot-toast", () => ({ default: { success: vi.fn() } }));
vi.mock("@/components/NoteDetailSidebar/NoteOutline", () => ({
  default: ({ headings }: { headings: unknown[] }) => <div data-testid="outline">{headings.length}</div>,
}));
vi.mock("@/components/NoteMetadata/Relation/useResolvedRelationNotes", () => ({ useResolvedRelationNotes: () => ({}) }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ profile: { instanceUrl: "https://notes.example/" } }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => currentUserState.value }));
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, values?: { source?: string }) => (values?.source ? `${key}:${values.source}` : key),
}));

const renderSidebar = (sidebar: React.ReactNode, route = "/notes/detail") =>
  render(<MemoryRouter initialEntries={[route]}>{sidebar}</MemoryRouter>);

const createIncomingReference = (noteName: string, sourceName = "notes/incoming", snippet = "Incoming backlink") =>
  create(NoteRelationSchema, {
    type: NoteRelation_Type.REFERENCE,
    note: create(NoteRelation_NoteRefSchema, { name: sourceName, snippet }),
    relatedNote: create(NoteRelation_NoteRefSchema, { name: noteName }),
  });

describe("NoteDetailSidebar", () => {
  beforeEach(() => {
    copyToClipboard.mockReset();
    currentUserState.value = { name: "users/alice" };
  });

  it("leaves returning to the origin to browser back and the app navigation", () => {
    const parentPage = "/spaces/travel/map?filter=tagSearch%3Atravel&lat=35&lng=135&zoom=12&note=notes%2Fdetail";
    const { container } = renderSidebar(<NoteDetailSidebar note={create(NoteSchema, { name: "notes/detail" })} parentPage={parentPage} />);

    expect(container.querySelector(`a[href="${parentPage}"]`)).toBeNull();
  });

  it("organizes source, in-page navigation, connections, and frequent actions without repeating metadata", async () => {
    const note = create(NoteSchema, {
      name: "notes/detail",
      creator: "users/alice",
      state: State.NORMAL,
      visibility: Visibility.PUBLIC,
      content: "# Overview\n\n## Details",
      tags: ["release/0.30"],
      location: create(LocationSchema, { placeholder: "Singapore" }),
      attachments: [
        create(AttachmentSchema, {
          name: "attachments/spec",
          filename: "release-notes.pdf",
          type: "application/pdf",
          size: 2048n,
        }),
      ],
      relations: [
        create(NoteRelationSchema, {
          type: NoteRelation_Type.REFERENCE,
          note: create(NoteRelation_NoteRefSchema, { name: "notes/detail" }),
          relatedNote: create(NoteRelation_NoteRefSchema, { name: "notes/outgoing", snippet: "Outgoing reference" }),
        }),
        createIncomingReference("notes/detail"),
      ],
    });
    const parentNote = create(NoteSchema, { name: "notes/parent", content: "Parent context" });
    const onEdit = vi.fn();
    const onCommentsOpen = vi.fn();
    const onCommentCreate = vi.fn();

    renderSidebar(
      <NoteDetailSidebar
        note={note}
        parentNote={parentNote}
        parentPage="/explore?filter=tagSearch%3Awork"
        commentCount={3}
        onEdit={onEdit}
        onCommentsOpen={onCommentsOpen}
        onCommentCreate={onCommentCreate}
      />,
    );

    expect(screen.getByText("note.on-this-note")).toBeInTheDocument();
    expect(screen.getByTestId("outline")).toHaveTextContent("2");
    expect(screen.getByRole("link", { name: /note.comment.self/ })).toHaveTextContent("3");
    expect(screen.getByRole("link", { name: /note.comment.self/ })).toHaveAttribute("href", "#note-comments");
    expect(screen.getByText("note.connections")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "note.parent-note: Parent context" })).toHaveAttribute("href", "/notes/parent");
    expect(screen.getByRole("link", { name: "common.referenced-by: Incoming backlink" })).toHaveAttribute("href", "/notes/incoming");
    expect(screen.queryByText("Outgoing reference")).not.toBeInTheDocument();
    expect(screen.getByText("common.actions")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("link", { name: /note.comment.self/ }));
    expect(onCommentsOpen).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "common.edit" }));
    expect(onEdit).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "note.comment.write-a-comment" }));
    expect(onCommentCreate).toHaveBeenCalledTimes(1);

    expect(screen.queryByRole("button", { name: "common.pin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.unpin" })).not.toBeInTheDocument();
    expect(screen.queryByText("common.visibility")).not.toBeInTheDocument();
    expect(screen.queryByText("common.created-at")).not.toBeInTheDocument();
    expect(screen.queryByText("common.location")).not.toBeInTheDocument();
    expect(screen.queryByText("common.attachments")).not.toBeInTheDocument();
    expect(screen.queryByText("release-notes.pdf")).not.toBeInTheDocument();
    expect(screen.queryByText("release/0.30")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.share" })).not.toBeInTheDocument();
  });

  it("lets an authenticated visitor comment while reserving edit for the author", () => {
    currentUserState.value = { name: "users/bob" };
    const note = create(NoteSchema, {
      name: "notes/public",
      creator: "users/alice",
      state: State.NORMAL,
      visibility: Visibility.PUBLIC,
      content: "Body",
      relations: [createIncomingReference("notes/public")],
    });

    renderSidebar(<NoteDetailSidebar note={note} parentPage="/explore" commentCount={1} onEdit={vi.fn()} onCommentCreate={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "common.edit" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "note.comment.write-a-comment" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "common.referenced-by: Incoming backlink" })).toBeInTheDocument();
  });

  it("keeps a simple note useful without rendering empty navigation or connections", () => {
    const note = create(NoteSchema, {
      name: "notes/simple",
      creator: "users/alice",
      state: State.NORMAL,
      content: "Body",
    });

    renderSidebar(<NoteDetailSidebar note={note} parentPage="/" commentCount={0} onEdit={vi.fn()} onCommentCreate={vi.fn()} />);

    expect(screen.queryByText("note.on-this-note")).not.toBeInTheDocument();
    expect(screen.queryByText("note.connections")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.edit" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "note.comment.write-a-comment" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.share" })).not.toBeInTheDocument();
  });

  it("keeps archived note navigation and comments readable without offering write actions", () => {
    const note = create(NoteSchema, {
      name: "notes/archived",
      creator: "users/alice",
      state: State.ARCHIVED,
      content: "Archived body",
    });

    renderSidebar(<NoteDetailSidebar note={note} parentPage="/archived" commentCount={2} onEdit={vi.fn()} onCommentCreate={vi.fn()} />);

    expect(screen.getByRole("link", { name: /note.comment.self/ })).toHaveTextContent("2");
    expect(screen.queryByRole("button", { name: "common.edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "note.comment.write-a-comment" })).not.toBeInTheDocument();
    // With nothing to do, the Actions section is left out instead of showing an empty heading.
    expect(screen.queryByText("common.actions")).not.toBeInTheDocument();
  });

  it("keeps shared notes private to their token context", () => {
    const note = create(NoteSchema, {
      name: "notes/shared",
      creator: "users/alice",
      state: State.NORMAL,
      visibility: Visibility.PUBLIC,
      content: "# Overview\n\nBody\n\n## Details\n\nMore",
      parent: "notes/parent",
      relations: [createIncomingReference("notes/shared", "notes/private", "Private backlink")],
    });
    const parentNote = create(NoteSchema, { name: "notes/parent", content: "Private parent" });

    renderSidebar(
      <NoteDetailSidebar
        note={note}
        parentNote={parentNote}
        parentPage="/explore"
        commentCount={4}
        forceReadonly
        onEdit={vi.fn()}
        onCommentCreate={vi.fn()}
      />,
      "/notes/shares/share-token",
    );

    expect(screen.getByText("note.on-this-note")).toBeInTheDocument();
    expect(screen.getByTestId("outline")).toHaveTextContent("2");
    expect(screen.queryByRole("link", { name: /note.comment.self/ })).not.toBeInTheDocument();
    expect(screen.queryByText("note.connections")).not.toBeInTheDocument();
    expect(screen.queryByText("Private parent")).not.toBeInTheDocument();
    expect(screen.queryByText("Private backlink")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "note.comment.write-a-comment" })).not.toBeInTheDocument();
    expect(screen.queryByText("common.actions")).not.toBeInTheDocument();
  });
});

import { create } from "@bufbuild/protobuf";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NotePanelList } from "@/components/NotePanel/NotePanelList";
import type { NoteViewProps } from "@/components/NoteView/types";
import { NoteSchema } from "@/types/proto/api/note_service_pb";

const mocks = vi.hoisted(() => ({
  creatorUsername: undefined as string | undefined,
  cards: [] as NoteViewProps[],
  userNames: [] as string[],
}));
vi.mock("@/components/NoteView", () => ({
  default: (props: NoteViewProps) => {
    mocks.cards.push(props);
    return <p>{props.note.name}</p>;
  },
}));
vi.mock("@/components/NoteContent/MentionResolutionContext", () => ({
  MentionResolutionProvider: ({ userNames, children }: { userNames: string[]; children: ReactNode }) => {
    mocks.userNames = userNames;
    return children;
  },
}));
vi.mock("@/components/NoteEditor", () => ({ default: () => <div data-testid="editor" /> }));
vi.mock("@/contexts/NewNoteContext", () => ({ NewNoteProvider: ({ children }: { children: ReactNode }) => children }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ isUserSettingsInitialized: true }) }));
vi.mock("@/contexts/SpaceContext", () => ({ useSpaceContext: () => ({ creatorUsername: mocks.creatorUsername }) }));
vi.mock("@/contexts/ViewContext", () => ({ useView: () => ({ compactMode: false }) }));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const notes = [
  create(NoteSchema, { name: "notes/a", creator: "users/alice", pinned: true, reactions: [{ creator: "users/carol" }] }),
  create(NoteSchema, { name: "notes/b", creator: "users/bob" }),
];

beforeEach(() => {
  mocks.cards = [];
  mocks.userNames = [];
});

describe("note panel list", () => {
  it("attributes a mixed-author selection and resolves every author in one batch", () => {
    mocks.creatorUsername = undefined;
    render(<NotePanelList notes={notes} selectionKey="2026-09-24" />);
    expect(mocks.cards.map((card) => card.showCreator)).toEqual([true, true]);
    expect(mocks.userNames).toEqual(["users/alice", "users/carol", "users/bob"]);
  });

  it("never shows pins, since notes are not pinned", () => {
    mocks.creatorUsername = undefined;
    render(<NotePanelList notes={notes} selectionKey="2026-09-24" />);
    expect(mocks.cards.map((card) => card.showPinned)).toEqual([undefined, undefined]);
  });

  it("hides creators in a single creator's scope", () => {
    mocks.creatorUsername = "alice";
    render(<NotePanelList notes={notes} selectionKey="2026-09-24" />);
    expect(mocks.cards.map((card) => card.showCreator)).toEqual([false, false]);
    expect(mocks.userNames).toEqual(["users/carol"]);
  });

  it("says the selection is empty instead of showing a blank panel", () => {
    render(<NotePanelList notes={[]} selectionKey="2026-09-13" emptyText="No notes on this day" />);
    expect(screen.getByText("No notes on this day")).toBeInTheDocument();
  });
});

import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { buildAttachmentLibraryStats, useAttachmentLibraryStats } from "@/hooks/useAttachmentLibrary";
import {
  type Attachment,
  AttachmentSchema,
  MotionMediaFamily,
  MotionMediaRole,
  MotionMediaSchema,
} from "@/types/proto/api/attachment_service_pb";

const clients = vi.hoisted(() => ({ listAttachments: vi.fn() }));

vi.mock("@/connect", () => ({
  attachmentServiceClient: { listAttachments: clients.listAttachments },
}));

const attachment = (name: string, type: string, note?: string): Attachment =>
  create(AttachmentSchema, { name: `attachments/${name}`, filename: name, type, note });

describe("buildAttachmentLibraryStats", () => {
  beforeEach(() => clients.listAttachments.mockReset());

  it("counts linked attachment types without expanding live-photo pairs or unlinked uploads", () => {
    const still = attachment("live.jpg", "image/jpeg", "notes/one");
    still.motionMedia = create(MotionMediaSchema, {
      family: MotionMediaFamily.APPLE_LIVE_PHOTO,
      role: MotionMediaRole.STILL,
      groupId: "live-photo",
    });
    const video = attachment("live.mov", "video/quicktime", "notes/one");
    video.motionMedia = create(MotionMediaSchema, {
      family: MotionMediaFamily.APPLE_LIVE_PHOTO,
      role: MotionMediaRole.VIDEO,
      groupId: "live-photo",
    });

    expect(
      buildAttachmentLibraryStats([
        still,
        video,
        attachment("photo.png", "image/png", "notes/one"),
        attachment("recording.mp3", "audio/mpeg", "notes/one"),
        attachment("notes.pdf", "application/pdf", "notes/one"),
        attachment("unused.png", "image/png"),
      ]),
    ).toEqual({
      media: 2,
      // Audio has no section of its own, so it counts as a document.
      documents: 2,
    });
  });

  it("loads every page before exposing complete sidebar counts", async () => {
    clients.listAttachments
      .mockResolvedValueOnce({
        attachments: [attachment("photo.png", "image/png", "notes/one")],
        nextPageToken: "1000",
      })
      .mockResolvedValueOnce({
        attachments: [attachment("unused.pdf", "application/pdf")],
        nextPageToken: "",
      });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: PropsWithChildren) => createElement(QueryClientProvider, { client: queryClient }, children);
    const filter = 'creator == "users/team"';
    const { result } = renderHook(() => useAttachmentLibraryStats(filter), { wrapper });

    await waitFor(() => expect(result.current.isComplete).toBe(true));
    expect(result.current.stats).toEqual({ media: 1, documents: 0 });
    expect(clients.listAttachments).toHaveBeenCalledTimes(2);
    expect(clients.listAttachments.mock.calls.map(([request]) => request.filter)).toEqual([filter, filter]);
  });
});

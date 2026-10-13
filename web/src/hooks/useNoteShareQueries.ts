import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { noteServiceClient } from "@/connect";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import type { NoteShare } from "@/types/proto/api/note_service_pb";
import {
  CreateNoteShareRequestSchema,
  DeleteNoteShareRequestSchema,
  GetSharedNoteRequestSchema,
  ListNoteSharesRequestSchema,
  NoteShareSchema,
} from "@/types/proto/api/note_service_pb";

// Query keys factory for share-related cache management.
export const noteShareKeys = {
  all: ["note-shares"] as const,
  list: (noteName: string) => [...noteShareKeys.all, "list", noteName] as const,
  byShare: (shareToken: string) => [...noteShareKeys.all, "by-share", shareToken] as const,
};

/** Lists all active share links for a note (creator-only). */
export function useNoteShares(noteName: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: noteShareKeys.list(noteName),
    queryFn: async () => {
      const response = await noteServiceClient.listNoteShares(create(ListNoteSharesRequestSchema, { parent: noteName }));
      return response.noteShares;
    },
    enabled: options?.enabled ?? !!noteName,
  });
}

/** Creates a new share link for a note. */
export function useCreateNoteShare() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ noteName, expireTime }: { noteName: string; expireTime?: Date }) => {
      const noteShare = create(NoteShareSchema, {
        expireTime: expireTime ? timestampFromDate(expireTime) : undefined,
      });
      const response = await noteServiceClient.createNoteShare(create(CreateNoteShareRequestSchema, { parent: noteName, noteShare }));
      return response;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: noteShareKeys.list(variables.noteName) });
    },
  });
}

/** Revokes (deletes) a share link. */
export function useDeleteNoteShare() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, noteName }: { name: string; noteName: string }) => {
      await noteServiceClient.deleteNoteShare(create(DeleteNoteShareRequestSchema, { name }));
      return noteName;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: noteShareKeys.list(variables.noteName) });
    },
  });
}

/** Resolves a share token to its note. Used by the public SharedNote page. */
export function useSharedNote(shareToken: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: noteShareKeys.byShare(shareToken),
    queryFn: async () => {
      const note = await noteServiceClient.getSharedNote(create(GetSharedNoteRequestSchema, { shareToken }));
      return note;
    },
    enabled: options?.enabled ?? !!shareToken,
    retry: false, // Don't retry NOT_FOUND — the link is invalid or expired
  });
}

/**
 * Returns the share URL for a NoteShare resource.
 * The token is the last path segment of the share name (notes/{uid}/shares/{token}).
 */
export function getShareUrl(share: NoteShare): string {
  const token = share.name.split("/").pop() ?? "";
  return `${window.location.origin}/notes/shares/${token}`;
}

/**
 * Returns the token portion of a NoteShare resource name.
 * Format: notes/{note}/shares/{token}
 */
export function getShareToken(share: NoteShare): string {
  return share.name.split("/").pop() ?? "";
}

/** Rewrites attachment URLs to include a share token for unauthenticated access. */
export function withShareAttachmentLinks(attachments: Attachment[], token: string): Attachment[] {
  return attachments.map((a) => {
    if (a.externalLink) return a;
    return { ...a, externalLink: `${window.location.origin}/file/${a.name}/${a.filename}?share_token=${encodeURIComponent(token)}` };
  });
}

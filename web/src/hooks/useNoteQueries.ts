import { create } from "@bufbuild/protobuf";
import { FieldMaskSchema } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError } from "@connectrpc/connect";
import {
  type InfiniteData,
  type QueryClient,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { noteServiceClient } from "@/connect";
import { attachmentKeys } from "@/hooks/useAttachmentQueries";
import { userKeys } from "@/hooks/useUserQueries";
import { DEFAULT_LIST_NOTES_PAGE_SIZE } from "@/lib/constants";
import { shouldRetry } from "@/lib/query-client";
import type { ListNotesRequest, ListNotesResponse, Note } from "@/types/proto/api/note_service_pb";
import { ListNoteCommentsRequestSchema, ListNotesRequestSchema, NoteSchema } from "@/types/proto/api/note_service_pb";

// Query keys factory for consistent cache management
export const noteKeys = {
  all: ["notes"] as const,
  lists: () => [...noteKeys.all, "list"] as const,
  list: (filters: Partial<ListNotesRequest>) => [...noteKeys.lists(), filters] as const,
  details: () => [...noteKeys.all, "detail"] as const,
  detail: (name: string) => [...noteKeys.details(), name] as const,
  comments: (name: string) => [...noteKeys.all, "comments", name] as const,
  linkMetadata: (url: string) => [...noteKeys.all, "linkMetadata", url] as const,
};

export const noteDetailQueryOptions = (name: string) =>
  queryOptions({
    queryKey: noteKeys.detail(name),
    queryFn: async ({ client, signal }) => {
      try {
        return await noteServiceClient.getNote({ name }, { signal });
      } catch (error) {
        if (!isNoteUnavailableError(error)) throw error;
        // Store a content-free result, replacing any formerly readable detail.
        // Remove copies and snippets so collection fallbacks cannot revive them.
        discardUnavailableNote(client, name);
        return null;
      }
    },
    retry: shouldRetry,
    staleTime: 1000 * 10,
    // A previous denial cannot establish access on a new visit, even while fresh.
    refetchOnMount: (query) => (query.state.data === null ? "always" : true),
  });

export function isNoteUnavailableError(error: unknown): boolean {
  return error instanceof ConnectError && (error.code === Code.PermissionDenied || error.code === Code.NotFound);
}

function discardUnavailableNote(client: QueryClient, name: string) {
  const stripRelations = (note: Note): Note => {
    const relations = note.relations.filter((relation) => relation.note?.name !== name && relation.relatedNote?.name !== name);
    return relations.length === note.relations.length ? note : { ...note, relations };
  };
  const stripList = (data: ListNotesResponse): ListNotesResponse => {
    const notes = data.notes.filter((note) => note.name !== name).map(stripRelations);
    return notes.length === data.notes.length && notes.every((note, index) => note === data.notes[index]) ? data : { ...data, notes };
  };
  for (const [key, data] of client.getQueriesData<NoteCollectionQueryData | Note | null>({ queryKey: noteKeys.all })) {
    let next = data;
    if (isNoteListResponse(data)) next = stripList(data);
    else if (isInfiniteNoteListData(data)) {
      const pages = data.pages.map(stripList);
      if (pages.some((page, index) => page !== data.pages[index])) next = { ...data, pages };
    } else if (key[1] === "detail" && data && "name" in data && data.name !== name) next = stripRelations(data);
    if (next !== data) client.setQueryData(key, next);
  }
}

type NotePatch = Partial<Note> & Pick<Note, "name">;
type NoteCollectionQueryData = ListNotesResponse | InfiniteData<ListNotesResponse>;

function isNoteListResponse(data: unknown): data is ListNotesResponse {
  return typeof data === "object" && data !== null && Array.isArray((data as { notes?: unknown }).notes);
}

function isInfiniteNoteListData(data: unknown): data is InfiniteData<ListNotesResponse> {
  return typeof data === "object" && data !== null && Array.isArray((data as { pages?: unknown }).pages);
}

function patchNoteListResponse(response: ListNotesResponse, update: NotePatch): ListNotesResponse {
  let changed = false;
  const notes = response.notes.map((note) => {
    if (note.name !== update.name) {
      return note;
    }

    changed = true;
    return { ...note, ...update };
  });

  return changed ? { ...response, notes } : response;
}

function patchNoteListQueryData<T>(data: T | undefined, update: NotePatch): T | undefined {
  if (!data) {
    return data;
  }

  if (isNoteListResponse(data)) {
    return patchNoteListResponse(data, update) as T;
  }

  if (isInfiniteNoteListData(data)) {
    let changed = false;
    const pages = data.pages.map((page) => {
      const patchedPage = patchNoteListResponse(page, update);
      if (patchedPage !== page) {
        changed = true;
      }
      return patchedPage;
    });

    return (changed ? { ...data, pages } : data) as T;
  }

  return data;
}

function findNoteInListResponse(response: ListNotesResponse, name: string): Note | undefined {
  return response.notes.find((note) => note.name === name);
}

function findNoteInQueryData(data: unknown, name: string): Note | undefined {
  if (!data) {
    return undefined;
  }

  if (isNoteListResponse(data)) {
    return findNoteInListResponse(data, name);
  }

  if (isInfiniteNoteListData(data)) {
    for (const page of data.pages) {
      const note = findNoteInListResponse(page, name);
      if (note) {
        return note;
      }
    }
  }

  return undefined;
}

export function findNoteInCollectionQueries(queryClient: QueryClient, name: string, freshOnly = false): Note | undefined {
  if (queryClient.getQueryData(noteKeys.detail(name)) === null) return undefined;
  for (const [key, data] of queryClient.getQueriesData<unknown>({ queryKey: noteKeys.all })) {
    const state = queryClient.getQueryState(key);
    if (freshOnly && (!state || state.isInvalidated || state.dataUpdatedAt < Date.now() - 10_000)) continue;
    const note = findNoteInQueryData(data, name);
    if (note) {
      return note;
    }
  }

  return undefined;
}

function patchNoteInCollectionQueries(queryClient: QueryClient, update: NotePatch) {
  queryClient.setQueriesData<NoteCollectionQueryData>({ queryKey: noteKeys.all }, (data) => patchNoteListQueryData(data, update));
}

export function useNotes(request: Partial<ListNotesRequest> = {}) {
  return useQuery({
    queryKey: noteKeys.list(request),
    queryFn: async () => {
      const response = await noteServiceClient.listNotes(create(ListNotesRequestSchema, request as Record<string, unknown>));
      return response;
    },
  });
}

export function useInfiniteNotes(request: Partial<ListNotesRequest> = {}, options?: { enabled?: boolean }) {
  return useInfiniteQuery({
    queryKey: noteKeys.list(request),
    queryFn: async ({ pageParam, signal }) => {
      const response = await noteServiceClient.listNotes(
        create(ListNotesRequestSchema, {
          ...request,
          pageToken: pageParam || "",
        } as Record<string, unknown>),
        { signal },
      );
      return response;
    },
    initialPageParam: "",
    getNextPageParam: (lastPage) => lastPage.nextPageToken || undefined,
    staleTime: 1000 * 60,
    gcTime: 1000 * 60 * 5,
    enabled: options?.enabled ?? true,
  });
}

export function useNote(name: string, options?: { enabled?: boolean }) {
  const query = useQuery({
    ...noteDetailQueryOptions(name),
    enabled: options?.enabled ?? true,
  });
  return { ...query, data: query.data ?? undefined, isUnavailable: query.data === null };
}

function isHTTPURL(url: string): boolean {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function useLinkMetadata(url: string, options?: { enabled?: boolean }) {
  const trimmedUrl = url.trim();

  return useQuery({
    queryKey: noteKeys.linkMetadata(trimmedUrl),
    queryFn: async () => {
      const metadata = await noteServiceClient.getLinkMetadata({ url: trimmedUrl });
      return metadata;
    },
    enabled: (options?.enabled ?? true) && isHTTPURL(trimmedUrl),
    staleTime: 1000 * 60 * 60 * 24,
    gcTime: 1000 * 60 * 60 * 24,
  });
}

export function useCreateNote() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (noteToCreate: Note) => {
      const note = await noteServiceClient.createNote({ note: noteToCreate });
      return note;
    },
    onSuccess: (newNote) => {
      // Invalidate note lists to refetch
      queryClient.invalidateQueries({ queryKey: noteKeys.lists() });
      // Add new note to cache
      queryClient.setQueryData(noteKeys.detail(newNote.name), newNote);
      // Invalidate user stats
      queryClient.invalidateQueries({ queryKey: userKeys.stats() });
      // Creating a note can bind previously unlinked attachments.
      queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() });
    },
  });
}

export function useUpdateNote() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ update, updateMask }: { update: Partial<Note>; updateMask: string[] }) => {
      const note = await noteServiceClient.updateNote({
        note: create(NoteSchema, update as Record<string, unknown>),
        updateMask: create(FieldMaskSchema, { paths: updateMask }),
      });
      return note;
    },
    onMutate: async ({ update, updateMask }) => {
      if (updateMask.includes("space")) return { previousNote: undefined };
      if (!update.name) {
        return { previousNote: undefined };
      }

      // Cancel outgoing refetches to prevent race conditions
      await queryClient.cancelQueries({ queryKey: noteKeys.all });

      // Snapshot previous value for rollback on error
      const previousNote =
        queryClient.getQueryData<Note>(noteKeys.detail(update.name)) || findNoteInCollectionQueries(queryClient, update.name);
      const notePatch: NotePatch = { ...update, name: update.name };

      // Optimistically update the cache
      if (previousNote) {
        queryClient.setQueryData(noteKeys.detail(update.name), { ...previousNote, ...notePatch });
      }
      patchNoteInCollectionQueries(queryClient, notePatch);

      return { previousNote };
    },
    onError: (_err, { update }, context) => {
      // Rollback on error
      if (context?.previousNote && update.name) {
        queryClient.setQueryData(noteKeys.detail(update.name), context.previousNote);
        patchNoteInCollectionQueries(queryClient, context.previousNote);
      } else {
        queryClient.invalidateQueries({ queryKey: noteKeys.all });
      }
    },
    onSuccess: (updatedNote, { updateMask }) => {
      if (updateMask.includes("space")) {
        queryClient.invalidateQueries({ queryKey: noteKeys.all });
      }
      // Update cache with server response
      queryClient.setQueryData(noteKeys.detail(updatedNote.name), updatedNote);
      patchNoteInCollectionQueries(queryClient, updatedNote);
      // Invalidate lists to refresh
      queryClient.invalidateQueries({ queryKey: noteKeys.lists() });
      if (updatedNote.parent) {
        queryClient.invalidateQueries({ queryKey: noteKeys.comments(updatedNote.parent) });
      }
      // Invalidate user stats
      queryClient.invalidateQueries({ queryKey: userKeys.stats() });
      // Placement changes move linked attachments between scoped libraries.
      queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() });
    },
  });
}

export function useDeleteNote() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (name: string) => {
      await noteServiceClient.deleteNote({ name });
      return name;
    },
    onSuccess: (name) => {
      // Remove from cache
      queryClient.removeQueries({ queryKey: noteKeys.detail(name) });
      // Invalidate lists
      queryClient.invalidateQueries({ queryKey: noteKeys.lists() });
      // Invalidate user stats
      queryClient.invalidateQueries({ queryKey: userKeys.stats() });
      // Note deletion can remove or unlink associated attachments.
      queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() });
    },
  });
}

export function useNoteComments(name: string, options?: { enabled?: boolean; pageSize?: number }) {
  return useQuery({
    queryKey: [...noteKeys.comments(name), options?.pageSize ?? 0],
    queryFn: async () => {
      const response = await noteServiceClient.listNoteComments(
        create(ListNoteCommentsRequestSchema, {
          parent: name,
          pageSize: options?.pageSize ?? 0,
        }),
      );
      return response;
    },
    enabled: options?.enabled ?? true,
    staleTime: 1000 * 60, // 1 minute
  });
}

// useInfiniteNoteComments paginates through every comment via nextPageToken, instead of
// stopping at the server's default page size (the cause of comments being truncated to 10).
export function useInfiniteNoteComments(name: string, options?: { enabled?: boolean; pageSize?: number }) {
  const pageSize = options?.pageSize ?? DEFAULT_LIST_NOTES_PAGE_SIZE;
  return useInfiniteQuery({
    queryKey: [...noteKeys.comments(name), "infinite", pageSize],
    queryFn: async ({ pageParam }) => {
      const response = await noteServiceClient.listNoteComments(
        create(ListNoteCommentsRequestSchema, {
          parent: name,
          pageSize,
          pageToken: pageParam || "",
        }),
      );
      return response;
    },
    initialPageParam: "",
    getNextPageParam: (lastPage) => lastPage.nextPageToken || undefined,
    select: (data) => data.pages.flatMap((page) => page.comments),
    enabled: options?.enabled ?? true,
    staleTime: 1000 * 60, // 1 minute
  });
}

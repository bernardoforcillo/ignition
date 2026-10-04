import { infiniteQueryOptions } from "@tanstack/react-query";

import { fileClient } from "~/lib/rpc";

const PAGE_SIZE = 25;

/** A workspace's ready files, newest first, a page at a time (keyset token from the server). */
export const filesQuery = (workspaceId: string) =>
	infiniteQueryOptions({
		queryKey: ["files", workspaceId],
		initialPageParam: "",
		queryFn: ({ pageParam }) =>
			fileClient.listFiles({
				workspaceId,
				pageSize: PAGE_SIZE,
				pageToken: pageParam,
			}),
		getNextPageParam: (last) => last.nextPageToken || undefined,
	});

/**
 * Registers a pending file and returns where to PUT its bytes. The browser uploads straight to
 * object storage with the returned URL and headers; the gateway never sees the bytes.
 */
export const createUpload = async (workspaceId: string, file: File) =>
	fileClient.createUpload({
		workspaceId,
		name: file.name,
		contentType: file.type || "application/octet-stream",
		sizeBytes: BigInt(file.size),
	});

export const completeUpload = async (workspaceId: string, fileId: string) =>
	fileClient.completeUpload({ workspaceId, fileId });

export const getDownloadUrl = async (workspaceId: string, fileId: string) =>
	(await fileClient.getDownloadUrl({ workspaceId, fileId })).url;

export const deleteFile = (workspaceId: string, fileId: string) =>
	fileClient.deleteFile({ workspaceId, fileId });

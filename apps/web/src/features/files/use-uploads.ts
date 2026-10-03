import { useCallback, useEffect, useRef, useState } from "react";

import { analytics } from "~/lib/analytics";
import { completeUpload, createUpload, deleteFile } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { putWithProgress, UploadError } from "~/lib/upload";

import { fileCategory } from "./format";

export type UploadStatus = "uploading" | "done" | "error";

export type UploadItem = {
	id: number;
	name: string;
	size: number;
	status: UploadStatus;
	/** 0..1 while uploading. */
	progress: number;
	/** A message safe to show, set when status is "error". */
	error?: string;
};

/** What the user is told when the bytes could not reach storage (the signed URL is never shown). */
export function uploadErrorMessage(error: unknown): string {
	if (error instanceof UploadError) {
		if (error.kind === "aborted") return "Upload canceled.";
		return error.kind === "network"
			? "The upload was interrupted. Check your connection and try again."
			: "The storage service refused the upload. Try again.";
	}
	return errorMessage(error, "files");
}

/**
 * Runs uploads for a workspace: register the file with the gateway, PUT the bytes straight to object
 * storage with progress, then confirm so the gateway verifies the object. A failure after the file
 * was registered gives its reserved bytes back at once (the gateway's cleanup job is the backstop).
 * Leaving the page cancels what is still in flight.
 */
export function useUploads(workspaceId: string, onUploaded: () => void) {
	const [items, setItems] = useState<UploadItem[]>([]);
	const nextId = useRef(1);
	const controllers = useRef(new Map<number, AbortController>());
	const onUploadedRef = useRef(onUploaded);
	onUploadedRef.current = onUploaded;

	const patch = useCallback((id: number, change: Partial<UploadItem>) => {
		setItems((list) =>
			list.map((item) => (item.id === id ? { ...item, ...change } : item)),
		);
	}, []);

	useEffect(() => {
		const active = controllers.current;
		return () => {
			for (const controller of active.values()) controller.abort();
			active.clear();
		};
	}, []);

	const run = useCallback(
		async (id: number, file: File, signal: AbortSignal) => {
			let registered: string | undefined;
			try {
				const upload = await createUpload(workspaceId, file);
				registered = upload.file?.id;
				await putWithProgress(
					{
						url: upload.uploadUrl,
						method: upload.uploadMethod || "PUT",
						headers: upload.uploadHeaders,
					},
					file,
					{ signal, onProgress: (progress) => patch(id, { progress }) },
				);
				await completeUpload(workspaceId, upload.file?.id ?? "");
				analytics.track("file_uploaded", {
					location: "files",
					size_bytes: file.size,
					type_category: fileCategory(file.type),
				});
				patch(id, { status: "done", progress: 1 });
				onUploadedRef.current();
			} catch (error) {
				patch(id, { status: "error", error: uploadErrorMessage(error) });
				if (registered) {
					deleteFile(workspaceId, registered).catch(() => undefined);
				}
			} finally {
				controllers.current.delete(id);
			}
		},
		[workspaceId, patch],
	);

	const start = useCallback(
		(files: File[]) => {
			for (const file of files) {
				const id = nextId.current++;
				const controller = new AbortController();
				controllers.current.set(id, controller);
				setItems((list) => [
					...list,
					{
						id,
						name: file.name,
						size: file.size,
						status: "uploading",
						progress: 0,
					},
				]);
				void run(id, file, controller.signal);
			}
		},
		[run],
	);

	const cancel = useCallback((id: number) => {
		controllers.current.get(id)?.abort();
	}, []);

	const dismiss = useCallback((id: number) => {
		setItems((list) => list.filter((item) => item.id !== id));
	}, []);

	return { items, start, cancel, dismiss };
}

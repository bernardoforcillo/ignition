import { Alert, Card, EmptyState, Skeleton } from "@ignition/components";
import {
	useInfiniteQuery,
	useMutation,
	useQueryClient,
} from "@tanstack/react-query";

import { useCurrentWorkspace } from "~/features/workspace";
import { deleteFile, filesQuery, getDownloadUrl } from "~/lib/api";
import {
	errorMessage,
	isFilesDisabled,
	isPermissionDenied,
} from "~/lib/errors";
import { isHttpUrl } from "~/lib/redirect";

import { FileList, type FileRow } from "./file-list";
import { QuotaMeter } from "./quota-meter";
import { UploadCard } from "./upload-card";
import { useUploads } from "./use-uploads";

/** Starts a download of a signed URL; the storage response is an attachment, so the page stays put. */
const saveFromUrl = (url: string, filename: string) => {
	if (!isHttpUrl(url)) throw new Error("Unexpected download URL");
	const anchor = document.createElement("a");
	anchor.href = url;
	anchor.download = filename;
	anchor.rel = "noopener";
	document.body.appendChild(anchor);
	anchor.click();
	anchor.remove();
};

export function FilesPanel() {
	const { workspace } = useCurrentWorkspace();
	const workspaceId = workspace?.id ?? "";
	const queryClient = useQueryClient();

	const files = useInfiniteQuery({
		...filesQuery(workspaceId),
		enabled: Boolean(workspaceId),
	});
	const refresh = () =>
		queryClient.invalidateQueries({
			queryKey: filesQuery(workspaceId).queryKey,
		});

	const uploads = useUploads(workspaceId, refresh);
	const download = useMutation({
		mutationFn: async (file: FileRow) =>
			saveFromUrl(await getDownloadUrl(workspaceId, file.id), file.name),
	});
	const remove = useMutation({
		mutationFn: (file: FileRow) => deleteFile(workspaceId, file.id),
		onSettled: refresh,
	});

	const rows: FileRow[] = (files.data?.pages ?? []).flatMap((page) =>
		page.files.map((f) => ({
			id: f.id,
			name: f.name,
			contentType: f.contentType,
			sizeBytes: f.sizeBytes,
			createdAt: f.createdAt,
		})),
	);
	const firstPage = files.data?.pages[0];
	const actionError = download.error ?? remove.error;
	const busyId = download.isPending
		? download.variables?.id
		: remove.isPending
			? remove.variables?.id
			: undefined;

	let body: React.ReactNode;
	if (files.error && isFilesDisabled(files.error)) {
		body = (
			<EmptyState
				title="File storage is not enabled"
				description="This deployment isn't set up to store files."
			/>
		);
	} else if (files.error && isPermissionDenied(files.error)) {
		body = <Alert tone="danger">{errorMessage(files.error, "files")}</Alert>;
	} else if (files.error && !firstPage) {
		body = (
			<Alert tone="danger">
				{errorMessage(files.error, "files")}{" "}
				<button
					type="button"
					className="font-medium underline"
					onClick={() => files.refetch()}
				>
					Try again
				</button>
			</Alert>
		);
	} else if (files.isLoading || !firstPage) {
		body = (
			<div role="status" className="space-y-4">
				<span className="sr-only">Loading files…</span>
				<Skeleton className="h-24 w-full" />
				<Skeleton className="h-32 w-full" />
			</div>
		);
	} else {
		body = (
			<>
				<Card title="Storage">
					<QuotaMeter
						usedBytes={Number(firstPage.usedBytes)}
						quotaBytes={
							firstPage.quotaBytes === undefined
								? undefined
								: Number(firstPage.quotaBytes)
						}
					/>
				</Card>
				<UploadCard
					items={uploads.items}
					onSelect={uploads.start}
					onCancel={uploads.cancel}
					onDismiss={uploads.dismiss}
				/>
				<FileList
					files={rows}
					hasMore={Boolean(files.hasNextPage)}
					loadingMore={files.isFetchingNextPage}
					onLoadMore={() => files.fetchNextPage()}
					onDownload={(file) => download.mutate(file)}
					onDelete={(file) => remove.mutate(file)}
					busyId={busyId}
					actionError={
						actionError ? errorMessage(actionError, "files") : undefined
					}
				/>
			</>
		);
	}

	return (
		<div className="space-y-6">
			<h1 className="text-2xl font-semibold">Files</h1>
			{body}
		</div>
	);
}

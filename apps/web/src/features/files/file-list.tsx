import { Alert, Button, Card, EmptyState } from "@ignition/components";
import { useState } from "react";

import { formatBytes, formatDateTime } from "./format";

export type FileRow = {
	id: string;
	name: string;
	contentType: string;
	sizeBytes: bigint;
	createdAt: string;
};

type Props = {
	files: FileRow[];
	hasMore: boolean;
	loadingMore: boolean;
	onLoadMore: () => void;
	onDownload: (file: FileRow) => void;
	onDelete: (file: FileRow) => void;
	/** The file whose download or delete is in flight. */
	busyId?: string;
	/** An error from the last download or delete, shown above the list. */
	actionError?: string;
};

/** The workspace's files with download and a two-step delete (no browser confirm dialog). */
export function FileList({
	files,
	hasMore,
	loadingMore,
	onLoadMore,
	onDownload,
	onDelete,
	busyId,
	actionError,
}: Props) {
	const [confirming, setConfirming] = useState<string | null>(null);

	return (
		<Card title="Files">
			<div className="space-y-3">
				{actionError ? <Alert tone="danger">{actionError}</Alert> : null}
				{files.length === 0 ? (
					<EmptyState
						title="No files yet"
						description="Files you upload appear here."
					/>
				) : (
					<ul className="divide-y divide-line">
						{files.map((file) => {
							const busy = busyId === file.id;
							return (
								<li
									key={file.id}
									className="flex flex-wrap items-center justify-between gap-3 py-2.5"
								>
									<div className="min-w-0">
										<p className="truncate font-medium text-fg">{file.name}</p>
										<p className="text-xs">
											{formatBytes(Number(file.sizeBytes))}
											{" · "}
											{formatDateTime(file.createdAt)}
										</p>
									</div>
									<div className="flex items-center gap-2">
										{confirming === file.id ? (
											<>
												<Button
													variant="danger"
													type="button"
													loading={busy}
													aria-label={`Confirm delete ${file.name}`}
													onClick={() => {
														setConfirming(null);
														onDelete(file);
													}}
												>
													Confirm delete
												</Button>
												<Button
													variant="secondary"
													type="button"
													onClick={() => setConfirming(null)}
												>
													Cancel
												</Button>
											</>
										) : (
											<>
												<Button
													variant="secondary"
													type="button"
													loading={busy}
													aria-label={`Download ${file.name}`}
													onClick={() => onDownload(file)}
												>
													Download
												</Button>
												<Button
													variant="ghost"
													type="button"
													disabled={busy}
													aria-label={`Delete ${file.name}`}
													onClick={() => setConfirming(file.id)}
												>
													Delete
												</Button>
											</>
										)}
									</div>
								</li>
							);
						})}
					</ul>
				)}
				{hasMore ? (
					<Button
						variant="secondary"
						type="button"
						loading={loadingMore}
						onClick={onLoadMore}
					>
						Load more files
					</Button>
				) : null}
			</div>
		</Card>
	);
}

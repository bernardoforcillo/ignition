import { Alert, Button, Card } from "@ignition/components";
import { useId } from "react";

import { formatBytes } from "./format";
import type { UploadItem } from "./use-uploads";

type Props = {
	items: UploadItem[];
	onSelect: (files: File[]) => void;
	onCancel: (id: number) => void;
	onDismiss: (id: number) => void;
};

/** The file picker and the uploads in flight, each with its own progress, cancel and result. */
export function UploadCard({ items, onSelect, onCancel, onDismiss }: Props) {
	const inputId = useId();
	const finished = items.filter((i) => i.status === "done").length;
	return (
		<Card title="Upload">
			<div className="space-y-4">
				<div>
					<label
						htmlFor={inputId}
						className="block text-sm font-medium text-fg"
					>
						Upload files
					</label>
					<input
						id={inputId}
						type="file"
						multiple
						className="mt-1 block w-full text-sm text-fg-muted file:mr-3 file:cursor-pointer file:rounded-card file:border-0 file:bg-surface-muted file:px-3 file:py-2 file:text-sm file:font-medium file:text-fg hover:file:bg-line focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
						onChange={(event) => {
							const input = event.currentTarget;
							const files = Array.from(input.files ?? []);
							// Clear it so choosing the same file again fires another change.
							input.value = "";
							if (files.length > 0) onSelect(files);
						}}
					/>
				</div>

				{/* Announces results to screen readers without moving focus. */}
				<p role="status" className="sr-only">
					{finished > 0 ? `${finished} uploaded` : ""}
				</p>

				{items.length > 0 ? (
					<ul className="space-y-3">
						{items.map((item) => (
							<li key={item.id} className="space-y-1">
								<div className="flex items-center justify-between gap-3">
									<p className="min-w-0 truncate font-medium text-fg">
										{item.name}{" "}
										<span className="font-normal text-fg-muted">
											({formatBytes(item.size)})
										</span>
									</p>
									{item.status === "uploading" ? (
										<Button
											variant="ghost"
											type="button"
											aria-label={`Cancel upload of ${item.name}`}
											onClick={() => onCancel(item.id)}
										>
											Cancel
										</Button>
									) : (
										<Button
											variant="ghost"
											type="button"
											aria-label={`Dismiss ${item.name}`}
											onClick={() => onDismiss(item.id)}
										>
											Dismiss
										</Button>
									)}
								</div>
								{item.status === "uploading" ? (
									<progress
										className="h-2 w-full accent-brand-600"
										aria-label={`Uploading ${item.name}`}
										value={Math.round(item.progress * 100)}
										max={100}
									/>
								) : item.status === "done" ? (
									<p className="text-sm text-success">Uploaded</p>
								) : (
									<Alert tone="danger">{item.error}</Alert>
								)}
							</li>
						))}
					</ul>
				) : null}
			</div>
		</Card>
	);
}

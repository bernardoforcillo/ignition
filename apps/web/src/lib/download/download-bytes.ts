export interface DownloadDeps {
	createObjectURL: (blob: Blob) => string;
	revokeObjectURL: (url: string) => void;
	createAnchor: () => {
		href: string;
		download: string;
		click: () => void;
	};
}

const browserDeps = (): DownloadDeps => ({
	createObjectURL: (blob) => URL.createObjectURL(blob),
	revokeObjectURL: (url) => URL.revokeObjectURL(url),
	createAnchor: () => document.createElement("a"),
});

/** A server-suggested name is untrusted: keep a plain file name, never a path. */
export function safeFilename(name: string, fallback = "download"): string {
	const base = name.split(/[\\/]/).pop() ?? "";
	// biome-ignore lint/suspicious/noControlCharactersInRegex: stripping control chars is the point
	const cleaned = base.replace(/[\u0000-\u001f\u007f<>:"|?*]/g, "").trim();
	return cleaned.replace(/^\.+/, "") || fallback;
}

/** Saves bytes as a file through a temporary object URL (no server round trip). */
export function downloadBytes(
	data: Uint8Array<ArrayBuffer>,
	filename: string,
	mimeType = "application/json",
	deps: DownloadDeps = browserDeps(),
): void {
	const url = deps.createObjectURL(new Blob([data], { type: mimeType }));
	try {
		const anchor = deps.createAnchor();
		anchor.href = url;
		anchor.download = safeFilename(filename);
		anchor.click();
	} finally {
		deps.revokeObjectURL(url);
	}
}

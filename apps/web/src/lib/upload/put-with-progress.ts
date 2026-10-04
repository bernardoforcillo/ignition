import { isHttpUrl } from "~/lib/redirect";

/** A signed upload request as the gateway returned it: use it exactly as given. */
export interface UploadTarget {
	url: string;
	method: string;
	headers: Record<string, string>;
}

export type UploadFailure = "network" | "rejected" | "aborted";

/** Why a direct-to-storage upload did not finish; `status` is set when storage answered. */
export class UploadError extends Error {
	constructor(
		readonly kind: UploadFailure,
		readonly status?: number,
	) {
		super(`upload ${kind}${status ? ` (${status})` : ""}`);
		this.name = "UploadError";
	}
}

export interface PutOptions {
	/** Called with 0..1 as bytes leave the browser. */
	onProgress?: (fraction: number) => void;
	signal?: AbortSignal;
	/** Replaces `new XMLHttpRequest()`; tests pass a fake. */
	createXhr?: () => XMLHttpRequest;
}

/**
 * PUTs a file to a signed URL and reports progress. It uses XMLHttpRequest because `fetch` cannot
 * report upload progress. The headers are the ones the signature covers; the browser sets the
 * content length itself from the body, which the signature also binds. The signed URL is a credential
 * for its lifetime, so it is never logged or put in an error message.
 */
export function putWithProgress(
	target: UploadTarget,
	body: Blob,
	{
		onProgress,
		signal,
		createXhr = () => new XMLHttpRequest(),
	}: PutOptions = {},
): Promise<void> {
	return new Promise<void>((resolve, reject) => {
		if (!isHttpUrl(target.url)) {
			reject(new UploadError("rejected"));
			return;
		}
		if (signal?.aborted) {
			reject(new UploadError("aborted"));
			return;
		}
		const xhr = createXhr();
		const abort = () => xhr.abort();
		const settle = (done: () => void) => {
			signal?.removeEventListener("abort", abort);
			done();
		};

		xhr.open(target.method || "PUT", target.url);
		for (const [name, value] of Object.entries(target.headers)) {
			xhr.setRequestHeader(name, value);
		}
		xhr.upload.onprogress = (event) => {
			if (event.lengthComputable && event.total > 0) {
				onProgress?.(Math.min(1, event.loaded / event.total));
			}
		};
		xhr.onload = () =>
			settle(() => {
				if (xhr.status >= 200 && xhr.status < 300) {
					onProgress?.(1);
					resolve();
				} else {
					reject(new UploadError("rejected", xhr.status));
				}
			});
		xhr.onerror = () => settle(() => reject(new UploadError("network")));
		xhr.ontimeout = () => settle(() => reject(new UploadError("network")));
		xhr.onabort = () => settle(() => reject(new UploadError("aborted")));
		signal?.addEventListener("abort", abort, { once: true });
		xhr.send(body);
	});
}

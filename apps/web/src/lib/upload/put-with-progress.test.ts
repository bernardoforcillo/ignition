import { describe, expect, it, vi } from "vitest";

import { putWithProgress, UploadError } from "./put-with-progress";

/** The few XMLHttpRequest members the uploader touches. */
class FakeXhr {
	method = "";
	url = "";
	headers: Record<string, string> = {};
	body: unknown;
	status = 0;
	aborted = false;
	upload: { onprogress: ((e: ProgressEvent) => void) | null } = {
		onprogress: null,
	};
	onload: (() => void) | null = null;
	onerror: (() => void) | null = null;
	ontimeout: (() => void) | null = null;
	onabort: (() => void) | null = null;

	open(method: string, url: string) {
		this.method = method;
		this.url = url;
	}
	setRequestHeader(name: string, value: string) {
		this.headers[name] = value;
	}
	send(body: unknown) {
		this.body = body;
	}
	abort() {
		this.aborted = true;
		this.onabort?.();
	}
	progress(loaded: number, total: number, lengthComputable = true) {
		this.upload.onprogress?.({
			loaded,
			total,
			lengthComputable,
		} as ProgressEvent);
	}
	finish(status: number) {
		this.status = status;
		this.onload?.();
	}
}

const target = {
	url: "https://storage.test/bucket/key?X-Amz-Signature=abc",
	method: "PUT",
	headers: { "Content-Type": "text/plain" },
};

const start = (options: Parameters<typeof putWithProgress>[2] = {}) => {
	const xhr = new FakeXhr();
	const blob = new Blob(["hello"], { type: "text/plain" });
	const promise = putWithProgress(target, blob, {
		...options,
		createXhr: () => xhr as unknown as XMLHttpRequest,
	});
	return { xhr, blob, promise };
};

describe("putWithProgress", () => {
	it("sends the body with exactly the signed method, url and headers", async () => {
		const { xhr, blob, promise } = start();
		xhr.finish(200);
		await promise;
		expect(xhr.method).toBe("PUT");
		expect(xhr.url).toBe(target.url);
		expect(xhr.headers).toEqual({ "Content-Type": "text/plain" });
		expect(xhr.body).toBe(blob);
	});

	it("reports progress as a fraction and ends at 1", async () => {
		const onProgress = vi.fn();
		const { xhr, promise } = start({ onProgress });
		xhr.progress(25, 100);
		xhr.progress(100, 100);
		xhr.progress(5, 0); // no total: nothing to report
		xhr.progress(5, 100, false); // not computable: ignored
		xhr.finish(200);
		await promise;
		expect(onProgress.mock.calls.map(([f]) => f)).toEqual([0.25, 1, 1]);
	});

	it("rejects with the storage status when it refuses the upload", async () => {
		const { xhr, promise } = start();
		xhr.finish(403);
		await expect(promise).rejects.toMatchObject({
			kind: "rejected",
			status: 403,
		});
	});

	it("rejects as a network error when the request fails or times out", async () => {
		const a = start();
		a.xhr.onerror?.();
		await expect(a.promise).rejects.toMatchObject({ kind: "network" });
		const b = start();
		b.xhr.ontimeout?.();
		await expect(b.promise).rejects.toMatchObject({ kind: "network" });
	});

	it("aborts the request when the signal fires", async () => {
		const controller = new AbortController();
		const { xhr, promise } = start({ signal: controller.signal });
		controller.abort();
		await expect(promise).rejects.toMatchObject({ kind: "aborted" });
		expect(xhr.aborted).toBe(true);
	});

	it("does not even open a request when already aborted", async () => {
		const controller = new AbortController();
		controller.abort();
		const { xhr, promise } = start({ signal: controller.signal });
		await expect(promise).rejects.toBeInstanceOf(UploadError);
		expect(xhr.url).toBe("");
	});

	it("refuses a target that is not an http(s) URL", async () => {
		const xhr = new FakeXhr();
		await expect(
			putWithProgress({ ...target, url: "javascript:alert(1)" }, new Blob([]), {
				createXhr: () => xhr as unknown as XMLHttpRequest,
			}),
		).rejects.toMatchObject({ kind: "rejected" });
		expect(xhr.url).toBe("");
	});

	it("never puts the signed URL in an error message", async () => {
		const { xhr, promise } = start();
		xhr.finish(500);
		const error = await promise.catch((e: Error) => e);
		expect(String(error)).not.toContain("Signature");
		expect(String(error)).not.toContain("storage.test");
	});
});

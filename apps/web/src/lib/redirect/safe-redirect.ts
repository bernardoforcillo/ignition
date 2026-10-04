const ORIGIN = "http://ignition.invalid";

/**
 * Open-redirect guard for the `redirect` query param: only same-origin, relative paths survive.
 * `//evil.com`, `/\evil.com`, absolute URLs and `javascript:` all fall back to `fallback`.
 */
export function safeRedirect(value: unknown, fallback = "/app"): string {
	if (typeof value !== "string" || value.length === 0) return fallback;
	if (!value.startsWith("/") || value.startsWith("//")) return fallback;
	// Browsers treat backslashes as slashes and strip tabs/newlines inside URLs.
	// biome-ignore lint/suspicious/noControlCharactersInRegex: rejecting control chars is the point
	if (/[\\\u0000-\u001f\u007f]/.test(value)) return fallback;
	try {
		const url = new URL(value, ORIGIN);
		if (url.origin !== ORIGIN) return fallback;
		return `${url.pathname}${url.search}${url.hash}`;
	} catch {
		return fallback;
	}
}

/** Hosted checkout/portal URLs come from the server; only ever navigate to http(s) ones. */
export function isHttpUrl(value: string): boolean {
	try {
		const { protocol } = new URL(value);
		return protocol === "https:" || protocol === "http:";
	} catch {
		return false;
	}
}

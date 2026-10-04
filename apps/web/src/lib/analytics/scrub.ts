/**
 * Query parameters that carry a secret or a destination and must never reach an analytics
 * vendor: the emailed single-use tokens, and the post-login redirect.
 */
const SENSITIVE_PARAMS = ["token", "redirect", "code", "state"];

/** Properties PostHog fills with URLs. */
const URL_PROPERTIES = [
	"$current_url",
	"$referrer",
	"$initial_referrer",
	"$initial_current_url",
	"$session_entry_url",
	"$session_entry_referrer",
	"$prev_pageview_pathname",
	"$pathname",
];

/** Removes the sensitive query parameters (and the fragment) from a URL; leaves anything else. */
export function scrubUrl(raw: string): string {
	let url: URL;
	try {
		url = new URL(raw, "http://placeholder.invalid");
	} catch {
		return "";
	}
	for (const key of SENSITIVE_PARAMS) url.searchParams.delete(key);
	url.hash = "";
	const isRelative = !/^[a-z][a-z0-9+.-]*:/i.test(raw);
	if (isRelative) return `${url.pathname}${url.search}`;
	return url.toString();
}

/** Scrubs every URL-valued property of an event's property bag, in place of a copy. */
export function scrubProperties<T extends Record<string, unknown>>(
	properties: T | undefined,
): T | undefined {
	if (!properties) return properties;
	const out: Record<string, unknown> = { ...properties };
	for (const key of URL_PROPERTIES) {
		const value = out[key];
		if (typeof value === "string" && value !== "") out[key] = scrubUrl(value);
	}
	return out as T;
}

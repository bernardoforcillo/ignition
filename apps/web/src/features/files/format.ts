const UNITS = ["B", "KB", "MB", "GB", "TB"] as const;

/** 1536 -> "1.5 KB". Binary steps (1024), one decimal under 10, none above; negatives read as 0. */
export function formatBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < UNITS.length - 1) {
		value /= 1024;
		unit += 1;
	}
	const rounded = unit === 0 || value >= 10 ? Math.round(value) : round1(value);
	return `${rounded} ${UNITS[unit]}`;
}

const round1 = (n: number) => Math.round(n * 10) / 10;

/** Used bytes as a percentage of the quota, clamped to 0..100; no quota (unlimited) is 0. */
export function usagePercent(used: number, quota: number | undefined): number {
	if (quota === undefined || quota <= 0) return quota === 0 ? 100 : 0;
	return Math.min(100, Math.max(0, Math.round((used / quota) * 100)));
}

/** RFC 3339 -> "2 Oct 2026, 14:03"; empty or unparsable input yields an empty string. */
export function formatDateTime(value: string): string {
	const date = new Date(value);
	if (!value || Number.isNaN(date.getTime())) return "";
	return new Intl.DateTimeFormat(undefined, {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(date);
}

/**
 * A coarse, fixed vocabulary for analytics: the browser-declared type is client input, so only the
 * category ever leaves the page, never the raw string.
 */
export function fileCategory(contentType: string): string {
	const type = contentType.toLowerCase().split(";")[0]?.trim() ?? "";
	if (type.startsWith("image/")) return "image";
	if (type === "application/pdf") return "document";
	if (type.startsWith("text/") || type === "application/json") return "text";
	if (type === "application/zip") return "archive";
	return "other";
}

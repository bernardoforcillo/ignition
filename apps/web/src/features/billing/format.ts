/** RFC 3339 -> "2 Oct 2026"; empty or unparsable input yields null. */
export function formatDate(value: string): string | null {
	if (!value) return null;
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return null;
	return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(
		date,
	);
}

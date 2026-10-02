/** "pro" -> "Pro", "extra_seats" -> "Extra seats": catalog ids carry no display names. */
export function displayName(id: string): string {
	const spaced = id.replace(/[-_]+/g, " ").trim();
	return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

/** RFC 3339 -> "2 Oct 2026"; empty or unparsable input yields null. */
export function formatDate(value: string): string | null {
	if (!value) return null;
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return null;
	return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(
		date,
	);
}

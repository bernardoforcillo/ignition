/** "pro" -> "Pro", "extra_seats" -> "Extra seats": catalog ids carry no display names. */
export function displayName(id: string): string {
	const spaced = id.replace(/[-_]+/g, " ").trim();
	return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

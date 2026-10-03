export type Heading = { id: string; text: string; level: 2 | 3 };

/** "Password policy" -> "password-policy": the anchor for a heading. */
export function headingId(text: string): string {
	return text
		.toLowerCase()
		.replace(/`/g, "")
		.replace(/[^\p{L}\p{Nd}]+/gu, "-")
		.replace(/^-+|-+$/g, "");
}

/** Level 2 and 3 headings of a Markdown source, skipping fenced code, for "On this page". */
export function extractHeadings(markdown: string): Heading[] {
	const headings: Heading[] = [];
	let fenced = false;
	for (const line of markdown.split("\n")) {
		if (/^\s*(```|~~~)/.test(line)) {
			fenced = !fenced;
			continue;
		}
		if (fenced) continue;
		const match = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line);
		if (!match?.[1] || !match[2]) continue;
		const text = match[2].replace(/`/g, "");
		headings.push({
			id: headingId(text),
			text,
			level: match[1].length === 2 ? 2 : 3,
		});
	}
	return headings;
}

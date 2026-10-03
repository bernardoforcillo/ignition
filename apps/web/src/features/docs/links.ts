export type DocLink =
	| { kind: "docs"; slug: string; hash: string | null }
	| { kind: "external"; href: string }
	| { kind: "blocked" };

/**
 * Markdown is content, not code, so its links are checked: only another docs page (`/docs/<slug>`,
 * optionally `#anchor`) or an https URL is followed. Anything else (javascript:, data:, http:,
 * protocol-relative or other app paths) renders as plain text.
 */
export function classifyLink(href: string | undefined): DocLink {
	if (!href) return { kind: "blocked" };
	const docs = /^\/docs\/([a-z0-9-]+)(?:#([A-Za-z0-9_-]+))?$/.exec(href);
	if (docs?.[1]) return { kind: "docs", slug: docs[1], hash: docs[2] ?? null };
	try {
		const url = new URL(href);
		if (url.protocol === "https:" && url.hostname) {
			return { kind: "external", href: url.href };
		}
	} catch {
		// not an absolute URL
	}
	return { kind: "blocked" };
}

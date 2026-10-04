/** One lazily loaded chunk per page: a visitor downloads only the page they read. */
const sources = import.meta.glob<string>("/src/docs/*.md", {
	query: "?raw",
	import: "default",
});

export async function loadDoc(slug: string): Promise<string | null> {
	const load = sources[`/src/docs/${slug}.md`];
	return load ? await load() : null;
}

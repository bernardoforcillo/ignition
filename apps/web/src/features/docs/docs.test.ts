import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import { extractHeadings, headingId } from "./headings";
import { classifyLink } from "./links";
import { DOCS, findDoc, neighbours } from "./manifest";

// Eager and raw, so the test reads exactly the files the app lazy-loads.
const files = import.meta.glob<string>("/src/docs/*.md", {
	query: "?raw",
	import: "default",
	eager: true,
});
const sourceOf = (slug: string) => files[`/src/docs/${slug}.md`];

describe("docs manifest", () => {
	it("has unique slugs", () => {
		const slugs = DOCS.map((doc) => doc.slug);
		expect(new Set(slugs).size).toBe(slugs.length);
	});

	it("has a Markdown file for every entry and an entry for every file", () => {
		for (const doc of DOCS) {
			expect(
				sourceOf(doc.slug),
				`missing src/docs/${doc.slug}.md`,
			).toBeTruthy();
		}
		const fileSlugs = Object.keys(files)
			.map((path) => path.replace("/src/docs/", "").replace(".md", ""))
			.sort();
		expect(fileSlugs).toEqual(DOCS.map((doc) => doc.slug).sort());
	});

	it("starts each page with a single h1 and gives every entry a title and summary", () => {
		for (const doc of DOCS) {
			const h1 = (sourceOf(doc.slug) ?? "").match(/^# .+$/gm) ?? [];
			expect(h1, doc.slug).toHaveLength(1);
			expect(doc.title.length).toBeGreaterThan(0);
			expect(doc.summary.length).toBeGreaterThan(0);
		}
	});

	it("lists every docs page in the sitemap", () => {
		const sitemap = readFileSync(
			fileURLToPath(new URL("../../../public/sitemap.xml", import.meta.url)),
			"utf8",
		);
		expect(sitemap).toContain("<loc>https://example.com/</loc>");
		for (const doc of DOCS) {
			expect(sitemap).toContain(`/docs/${doc.slug}</loc>`);
		}
	});
});

describe("neighbours", () => {
	it("has no previous page for the first and no next page for the last", () => {
		const first = DOCS[0];
		const last = DOCS[DOCS.length - 1];
		expect(neighbours(first?.slug ?? "").previous).toBeNull();
		expect(neighbours(first?.slug ?? "").next).toBe(DOCS[1]);
		expect(neighbours(last?.slug ?? "").next).toBeNull();
		expect(neighbours(last?.slug ?? "").previous).toBe(DOCS[DOCS.length - 2]);
	});

	it("returns both sides for a page in the middle", () => {
		const middle = DOCS[2];
		expect(neighbours(middle?.slug ?? "")).toEqual({
			previous: DOCS[1],
			next: DOCS[3],
		});
	});

	it("returns nothing for an unknown slug", () => {
		expect(neighbours("nope")).toEqual({ previous: null, next: null });
		expect(findDoc("nope")).toBeUndefined();
	});
});

describe("classifyLink", () => {
	it("follows docs pages, with or without an anchor", () => {
		expect(classifyLink("/docs/api")).toEqual({
			kind: "docs",
			slug: "api",
			hash: null,
		});
		expect(classifyLink("/docs/authentication#password-policy")).toEqual({
			kind: "docs",
			slug: "authentication",
			hash: "password-policy",
		});
	});

	it("follows https URLs", () => {
		expect(classifyLink("https://example.com/a?b=1")).toEqual({
			kind: "external",
			href: "https://example.com/a?b=1",
		});
	});

	it("blocks everything else", () => {
		for (const href of [
			undefined,
			"",
			"javascript:alert(1)",
			"data:text/html,x",
			"http://example.com",
			"//example.com",
			"/signup",
			"/docs/../app",
			"/docs/",
			"mailto:a@b.c",
			"#local",
		]) {
			expect(classifyLink(href), String(href)).toEqual({ kind: "blocked" });
		}
	});
});

describe("links inside the Markdown files", () => {
	const linkPattern = /\]\(([^)\s]+)\)/g;

	it("only point at existing docs pages and anchors, or at https URLs", () => {
		for (const doc of DOCS) {
			const source = sourceOf(doc.slug) ?? "";
			for (const [, href] of source.matchAll(linkPattern)) {
				const link = classifyLink(href);
				expect(link.kind, `${doc.slug}: ${href}`).not.toBe("blocked");
				if (link.kind !== "docs") continue;
				const target = findDoc(link.slug);
				expect(target, `${doc.slug}: ${href} has no page`).toBeDefined();
				if (link.hash) {
					const ids = extractHeadings(sourceOf(link.slug) ?? "").map(
						(h) => h.id,
					);
					expect(ids, `${doc.slug}: ${href} has no such heading`).toContain(
						link.hash,
					);
				}
			}
		}
	});

	it("contain no raw HTML", () => {
		for (const doc of DOCS) {
			const outsideCode = (sourceOf(doc.slug) ?? "")
				.replace(/```[\s\S]*?```/g, "")
				.replace(/`[^`]*`/g, "");
			expect(outsideCode, doc.slug).not.toMatch(/<\/?[a-z][^>]*>/i);
		}
	});
});

describe("headings", () => {
	it("builds anchors from the heading text", () => {
		expect(headingId("Password policy")).toBe("password-policy");
		expect(headingId("Calls that need a `token`")).toBe(
			"calls-that-need-a-token",
		);
	});

	it("lists level 2 and 3 headings and ignores fenced code", () => {
		const headings = extractHeadings(
			"# Title\n\n## One\n\n```sh\n## not a heading\n```\n\n### Two\n",
		);
		expect(headings).toEqual([
			{ id: "one", text: "One", level: 2 },
			{ id: "two", text: "Two", level: 3 },
		]);
	});
});

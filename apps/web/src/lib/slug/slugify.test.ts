import { describe, expect, it } from "vitest";

import { slugify } from "./slugify";

describe("slugify", () => {
	it("lowercases and hyphenates words", () => {
		expect(slugify("Acme Inc")).toBe("acme-inc");
	});

	it("collapses punctuation and trims hyphens", () => {
		expect(slugify("  --Acme,  Inc.!  ")).toBe("acme-inc");
	});

	it("strips accents", () => {
		expect(slugify("Café Münch")).toBe("cafe-munch");
	});

	it("returns an empty string when nothing usable remains", () => {
		expect(slugify("!!!")).toBe("");
	});

	it("caps the length without leaving a trailing hyphen", () => {
		const slug = slugify(`${"a".repeat(39)} bbbb`);
		expect(slug.length).toBeLessThanOrEqual(40);
		expect(slug.endsWith("-")).toBe(false);
	});
});

import { describe, expect, it } from "vitest";

import { isHttpUrl, safeRedirect } from "./safe-redirect";

describe("safeRedirect", () => {
	it("keeps same-origin relative paths with query and hash", () => {
		expect(safeRedirect("/app/members")).toBe("/app/members");
		expect(safeRedirect("/invite/accept?token=abc")).toBe(
			"/invite/accept?token=abc",
		);
		expect(safeRedirect("/app#top")).toBe("/app#top");
	});

	it("rejects protocol-relative URLs", () => {
		expect(safeRedirect("//evil.com")).toBe("/app");
		expect(safeRedirect("//evil.com/app")).toBe("/app");
	});

	it("rejects absolute URLs and script URLs", () => {
		expect(safeRedirect("https://evil.com")).toBe("/app");
		expect(safeRedirect("http://localhost/app")).toBe("/app");
		expect(safeRedirect("javascript:alert(1)")).toBe("/app");
	});

	it("rejects backslash and control-character tricks", () => {
		expect(safeRedirect("/\\evil.com")).toBe("/app");
		expect(safeRedirect("/\t/evil.com")).toBe("/app");
		expect(safeRedirect("/\n/evil.com")).toBe("/app");
	});

	it("rejects relative paths without a leading slash and non-strings", () => {
		expect(safeRedirect("app")).toBe("/app");
		expect(safeRedirect(undefined)).toBe("/app");
		expect(safeRedirect(42)).toBe("/app");
		expect(safeRedirect("")).toBe("/app");
	});

	it("uses the given fallback", () => {
		expect(safeRedirect("//evil.com", "/login")).toBe("/login");
	});
});

describe("isHttpUrl", () => {
	it("accepts http(s) URLs only", () => {
		expect(isHttpUrl("https://checkout.example.com/s/1")).toBe(true);
		expect(isHttpUrl("javascript:alert(1)")).toBe(false);
		expect(isHttpUrl("data:text/html,x")).toBe(false);
		expect(isHttpUrl("/relative")).toBe(false);
	});
});

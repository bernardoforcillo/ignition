import { describe, expect, it } from "vitest";

import {
	fileCategory,
	formatBytes,
	formatDateTime,
	usagePercent,
} from "./format";

describe("formatBytes", () => {
	it.each([
		[0, "0 B"],
		[-5, "0 B"],
		[Number.NaN, "0 B"],
		[1, "1 B"],
		[1023, "1023 B"],
		[1024, "1 KB"],
		[1536, "1.5 KB"],
		[10 * 1024, "10 KB"],
		[100 * 1024 * 1024, "100 MB"],
		[26214400, "25 MB"],
		[10 * 1024 ** 3, "10 GB"],
		[5 * 1024 ** 4, "5 TB"],
		[5000 * 1024 ** 4, "5000 TB"],
	])("formats %d as %s", (bytes, expected) => {
		expect(formatBytes(bytes)).toBe(expected);
	});
});

describe("usagePercent", () => {
	it("is a clamped whole percentage of the quota", () => {
		expect(usagePercent(50, 200)).toBe(25);
		expect(usagePercent(300, 200)).toBe(100);
		expect(usagePercent(-1, 200)).toBe(0);
	});

	it("is 0 for an unlimited quota and full for a quota of zero", () => {
		expect(usagePercent(500, undefined)).toBe(0);
		expect(usagePercent(1, 0)).toBe(100);
	});
});

describe("formatDateTime", () => {
	it("formats a timestamp and tolerates garbage", () => {
		expect(formatDateTime("2026-10-03T09:00:00Z")).toMatch(/2026/);
		expect(formatDateTime("")).toBe("");
		expect(formatDateTime("not a date")).toBe("");
	});
});

describe("fileCategory", () => {
	it("maps a declared type to one of a few categories", () => {
		expect(fileCategory("image/png")).toBe("image");
		expect(fileCategory("application/pdf")).toBe("document");
		expect(fileCategory("text/plain; charset=utf-8")).toBe("text");
		expect(fileCategory("application/json")).toBe("text");
		expect(fileCategory("application/zip")).toBe("archive");
	});

	it("never passes an unknown or hostile type through", () => {
		expect(fileCategory("application/x-evil; token=abc")).toBe("other");
		expect(fileCategory("")).toBe("other");
	});
});

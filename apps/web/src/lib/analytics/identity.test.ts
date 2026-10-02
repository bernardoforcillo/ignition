import { describe, expect, it } from "vitest";

import { identityAction } from "./identity";

describe("identityAction", () => {
	it.each([
		["", "u1", "identify"],
		["u1", "u2", "identify"],
		["u1", "", "reset"],
		["u1", "u1", "none"],
		["", "", "none"],
	] as const)("%j -> %j is %s", (prev, next, want) => {
		expect(identityAction(prev, next)).toBe(want);
	});
});

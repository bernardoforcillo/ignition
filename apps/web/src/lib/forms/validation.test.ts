import { describe, expect, it } from "vitest";

import { newPasswordError } from "./validation";

describe("newPasswordError", () => {
	it.each([
		["too short", "Aa1!aaaa"],
		["no upper case", "correct-horse-9-battery!"],
		["no lower case", "CORRECT-HORSE-9-BATTERY!"],
		["no digit", "Correct-Horse-Battery!"],
		["no symbol", "CorrectHorse9Battery"],
		["padded with spaces", "Aa1         "],
	])("rejects a password that is %s", (_, password) => {
		expect(newPasswordError(password)).toMatch(/at least 12 characters/i);
	});

	it("accepts one that satisfies the server policy", () => {
		expect(newPasswordError("Correct-Horse-9-Battery!")).toBeUndefined();
	});
});

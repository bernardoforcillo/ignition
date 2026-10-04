import { describe, expect, it } from "vitest";

import { displayName } from "./display-name";

describe("displayName", () => {
	it("capitalises and spaces catalog ids", () => {
		expect(displayName("pro")).toBe("Pro");
		expect(displayName("extra-api-calls")).toBe("Extra api calls");
		expect(displayName("extra_seats")).toBe("Extra seats");
	});

	it("survives an empty id", () => {
		expect(displayName("")).toBe("");
	});
});

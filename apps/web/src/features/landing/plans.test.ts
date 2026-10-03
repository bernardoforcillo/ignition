import { describe, expect, it } from "vitest";

import { buildPricing } from "./plans";

describe("buildPricing", () => {
	it("always starts with the Free card, even when billing is disabled", () => {
		const { plans, addOns } = buildPricing([]);
		expect(plans.map((p) => p.id)).toEqual(["free"]);
		expect(plans[0]).toMatchObject({ name: "Free", price: "$0" });
		expect(addOns).toEqual([]);
	});

	it("adds a card per plan the API lists, with its marketing copy", () => {
		const { plans } = buildPricing([
			{ kind: "plan", id: "pro" },
			{ kind: "addon", id: "extra-api-calls" },
		]);
		expect(plans.map((p) => p.id)).toEqual(["free", "pro"]);
		expect(plans[1]).toMatchObject({
			name: "Pro",
			highlighted: true,
			ctaLabel: "Choose Pro",
		});
		expect(plans[1]?.bullets.length).toBeGreaterThan(0);
	});

	it("lists add-ons separately and never as plans", () => {
		const { plans, addOns } = buildPricing([
			{ kind: "addon", id: "extra-api-calls" },
		]);
		expect(plans.map((p) => p.id)).toEqual(["free"]);
		expect(addOns).toEqual([
			{
				id: "extra-api-calls",
				name: "Extra API calls",
				description: expect.stringContaining("50,000"),
			},
		]);
	});

	it("renders an unknown plan with a derived title and no price instead of failing", () => {
		const { plans, addOns } = buildPricing([
			{ kind: "plan", id: "team_plus" },
			{ kind: "addon", id: "extra-seats" },
		]);
		expect(plans[1]).toMatchObject({
			id: "team_plus",
			name: "Team plus",
			price: null,
			period: null,
			bullets: [],
			highlighted: false,
			ctaLabel: "Choose Team plus",
		});
		expect(addOns[0]).toMatchObject({ name: "Extra seats", description: "" });
	});

	it("shows one card per plan however many prices it has, and never a second Free", () => {
		const { plans } = buildPricing([
			{ kind: "plan", id: "pro" },
			{ kind: "plan", id: "pro" },
			{ kind: "plan", id: "free" },
		]);
		expect(plans.map((p) => p.id)).toEqual(["free", "pro"]);
	});

	it("orders plans with copy first, then the rest as listed", () => {
		const { plans } = buildPricing([
			{ kind: "plan", id: "zeta" },
			{ kind: "plan", id: "pro" },
			{ kind: "plan", id: "alpha" },
		]);
		expect(plans.map((p) => p.id)).toEqual(["free", "pro", "zeta", "alpha"]);
	});

	it("ignores kinds it does not know", () => {
		const { plans, addOns } = buildPricing([{ kind: "seat", id: "x" }]);
		expect(plans).toHaveLength(1);
		expect(addOns).toHaveLength(0);
	});
});

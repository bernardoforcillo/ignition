import { displayName } from "~/lib/format";

/**
 * Marketing copy for the pricing section, keyed by the plan ids of the features catalog
 * (`go-packages/features/catalog.go`). The API lists which plans can be bought but not what they
 * cost or promise, so the words live here. REPLACE the copy, and above all the price text, with
 * your own and keep it in step with what Stripe charges.
 */
export type PlanCopy = {
	name: string;
	tagline: string;
	/** Shown as written, e.g. "$29". Absent means no price is displayed. */
	price?: string;
	period?: string;
	bullets: readonly string[];
	highlighted?: boolean;
};

export const FREE_PLAN_ID = "free";

/** Insertion order is display order. */
export const PLAN_COPY: Readonly<Record<string, PlanCopy>> = {
	[FREE_PLAN_ID]: {
		name: "Free",
		tagline: "Everything you need to build and try the product.",
		price: "$0",
		period: "forever",
		bullets: [
			"Workspaces with owner, admin and member roles",
			"Invite teammates by email",
			"1,000 API calls per month",
			"Export or erase your data any time",
		],
	},
	pro: {
		name: "Pro",
		tagline: "Higher limits for workspaces that are growing.",
		price: "$29",
		period: "per month",
		highlighted: true,
		bullets: [
			"Everything in Free",
			"100,000 API calls per month",
			"Data export feature",
			"Manage billing in the customer portal",
		],
	},
};

export type AddOnCopy = { name: string; description: string };

export const ADD_ON_COPY: Readonly<Record<string, AddOnCopy>> = {
	"extra-api-calls": {
		name: "Extra API calls",
		description: "50,000 more API calls per month on top of Pro.",
	},
};

/** The part of the `listPrices` answer the landing page uses. */
export type ListedPrice = { kind: string; id: string };

export type PlanCard = {
	id: string;
	name: string;
	tagline: string;
	price: string | null;
	period: string | null;
	bullets: readonly string[];
	highlighted: boolean;
	ctaLabel: string;
};

export type AddOnCard = { id: string; name: string; description: string };

export type Pricing = { plans: PlanCard[]; addOns: AddOnCard[] };

const planCard = (id: string): PlanCard => {
	const copy = PLAN_COPY[id];
	const name = copy?.name ?? displayName(id);
	return {
		id,
		name,
		tagline: copy?.tagline ?? "",
		price: copy?.price ?? null,
		period: copy?.period ?? null,
		bullets: copy?.bullets ?? [],
		highlighted: copy?.highlighted ?? false,
		ctaLabel: id === FREE_PLAN_ID ? "Start for free" : `Choose ${name}`,
	};
};

const copyOrder = (id: string): number => {
	const index = Object.keys(PLAN_COPY).indexOf(id);
	return index === -1 ? Number.MAX_SAFE_INTEGER : index;
};

/**
 * Merges the purchasable catalog with the marketing copy. The Free card is always first and always
 * present (billing off means an empty list); one card per plan id however many prices it has;
 * plans without copy get a title derived from the id and no price, never an error. Stable order:
 * plans with copy as written above, then the rest as the API listed them.
 */
export function buildPricing(prices: readonly ListedPrice[]): Pricing {
	const planIds: string[] = [];
	const addOnIds: string[] = [];
	for (const { kind, id } of prices) {
		if (kind === "plan" && id !== FREE_PLAN_ID && !planIds.includes(id)) {
			planIds.push(id);
		}
		if (kind === "addon" && !addOnIds.includes(id)) addOnIds.push(id);
	}
	const paid = planIds
		.map((id, position) => ({ id, position }))
		.sort(
			(a, b) => copyOrder(a.id) - copyOrder(b.id) || a.position - b.position,
		)
		.map(({ id }) => planCard(id));

	return {
		plans: [planCard(FREE_PLAN_ID), ...paid],
		addOns: addOnIds.map((id) => ({
			id,
			name: ADD_ON_COPY[id]?.name ?? displayName(id),
			description: ADD_ON_COPY[id]?.description ?? "",
		})),
	};
}

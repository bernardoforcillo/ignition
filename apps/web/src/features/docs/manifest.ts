export type DocEntry = {
	/** URL segment: `/docs/<slug>`, and the file `src/docs/<slug>.md`. */
	slug: string;
	title: string;
	/** One sentence, used as the page's meta description. */
	summary: string;
};

/** The reading order of the docs. Adding a page means a file in `src/docs/` and a line here. */
export const DOCS: readonly DocEntry[] = [
	{
		slug: "overview",
		title: "Overview",
		summary: "What Ignition is, what is in the box and how the pieces fit.",
	},
	{
		slug: "getting-started",
		title: "Getting started",
		summary: "Sign up, verify your email and create your first workspace.",
	},
	{
		slug: "authentication",
		title: "Authentication and security",
		summary: "Password policy, sessions and the password reset flow.",
	},
	{
		slug: "workspaces",
		title: "Workspaces and roles",
		summary: "Owner, admin and member roles, and how invitations work.",
	},
	{
		slug: "billing",
		title: "Plans and billing",
		summary:
			"Free and paid plans, checkout, the customer portal and cancellation.",
	},
	{
		slug: "your-data",
		title: "Your data",
		summary: "Exporting your data and erasing your account.",
	},
	{
		slug: "api",
		title: "API",
		summary: "Call the Connect API over HTTP and JSON with curl.",
	},
	{
		slug: "self-hosting",
		title: "Self-hosting",
		summary:
			"Environment variables, Postgres, Kubernetes manifests and health probes.",
	},
	{
		slug: "privacy",
		title: "Privacy and analytics",
		summary: "What is collected, when, and how consent works.",
	},
];

export const findDoc = (slug: string | undefined): DocEntry | undefined =>
	DOCS.find((doc) => doc.slug === slug);

export type Neighbours = { previous: DocEntry | null; next: DocEntry | null };

/** The pages before and after `slug` in reading order; both null for an unknown slug. */
export function neighbours(
	slug: string,
	docs: readonly DocEntry[] = DOCS,
): Neighbours {
	const index = docs.findIndex((doc) => doc.slug === slug);
	if (index === -1) return { previous: null, next: null };
	return {
		previous: docs[index - 1] ?? null,
		next: docs[index + 1] ?? null,
	};
}

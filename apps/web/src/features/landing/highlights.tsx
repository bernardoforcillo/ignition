type Highlight = { title: string; body: string };

const HIGHLIGHTS: readonly Highlight[] = [
	{
		title: "Accounts done right",
		body: "Email sign-up with verification, a real password policy, rotating refresh tokens and a reset flow that never reveals who has an account.",
	},
	{
		title: "Workspaces with roles",
		body: "Every customer gets a workspace with owner, admin and member roles, and invites teammates by email with a single-use link.",
	},
	{
		title: "Plans and metered limits",
		body: "A feature catalog of plans, add-ons and flags answers can this workspace use this, and how much is left, before you ship the feature.",
	},
	{
		title: "Stripe billing",
		body: "Hosted checkout, the customer portal and signed webhooks keep each workspace's plan in step with what Stripe says.",
	},
	{
		title: "GDPR-ready",
		body: "People can download their data and erase their account. Analytics are off until a visitor accepts, and hosted in the EU by default.",
	},
	{
		title: "Email from Go",
		body: "Templates are written once with react.email, exported to static HTML and sent only from the Go gateway. No email SDK in the browser.",
	},
];

export function Highlights() {
	return (
		<section
			id="features"
			aria-labelledby="features-title"
			className="border-b border-line"
		>
			<div className="mx-auto max-w-6xl px-4 py-16">
				<h2
					id="features-title"
					className="text-3xl font-semibold tracking-tight text-fg"
				>
					Everything a SaaS needs before its first customer
				</h2>
				<ul className="mt-10 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
					{HIGHLIGHTS.map((item) => (
						<li
							key={item.title}
							className="rounded-card border border-line bg-surface p-5 shadow-sm"
						>
							<h3 className="text-base font-semibold text-fg">{item.title}</h3>
							<p className="mt-2 text-sm text-fg-muted">{item.body}</p>
						</li>
					))}
				</ul>
			</div>
		</section>
	);
}

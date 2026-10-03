type Faq = { question: string; answer: string };

const FAQS: readonly Faq[] = [
	{
		question: "What is Ignition?",
		answer:
			"A starter for SaaS products: a React web app and a Go gateway that already handle accounts, workspaces, billing, email and data requests, so a new product begins with the boring parts finished.",
	},
	{
		question: "What do I get on the Free plan?",
		answer:
			"Every workspace starts on Free, with roles, invitations and a monthly allowance of metered calls. You can upgrade a workspace later from its billing page.",
	},
	{
		question: "How does billing work?",
		answer:
			"Paying happens on a Stripe hosted page and subscriptions are managed in the Stripe customer portal. Only workspace owners and admins can change a plan.",
	},
	{
		question: "What happens to my data if I leave?",
		answer:
			"You can download your data as JSON and delete your account from the settings page. Workspaces you own alone are erased with it; the docs explain the few cases that block a deletion.",
	},
	{
		question: "Do you track me?",
		answer:
			"Analytics stay off until you accept the banner, and they are hosted in the EU by default. Declining changes nothing about how the product works.",
	},
	{
		question: "Can I run it myself?",
		answer:
			"Yes. The gateway needs a Postgres database and a handful of environment variables, and the repository includes Kubernetes manifests. See the self-hosting guide in the docs.",
	},
];

export function FaqSection() {
	return (
		<section
			id="faq"
			aria-labelledby="faq-title"
			className="border-b border-line"
		>
			<div className="mx-auto max-w-3xl px-4 py-16">
				<h2
					id="faq-title"
					className="text-3xl font-semibold tracking-tight text-fg"
				>
					Frequently asked questions
				</h2>
				<div className="mt-8 divide-y divide-line rounded-card border border-line bg-surface">
					{FAQS.map((item) => (
						<details key={item.question} className="group px-5 py-4">
							<summary className="flex cursor-pointer list-none items-center justify-between gap-4 rounded-card text-base font-medium text-fg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-brand-500 [&::-webkit-details-marker]:hidden">
								{item.question}
								<span
									aria-hidden="true"
									className="text-fg-muted transition-transform group-open:rotate-45"
								>
									+
								</span>
							</summary>
							<p className="mt-3 text-sm text-fg-muted">{item.answer}</p>
						</details>
					))}
				</div>
			</div>
		</section>
	);
}

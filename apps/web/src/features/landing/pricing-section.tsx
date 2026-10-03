import { Alert, Badge, Skeleton } from "@ignition/components";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { linkButtonClass } from "~/features/site-chrome";
import { analytics } from "~/lib/analytics";
import { pricesQuery } from "~/lib/api";
import { isBillingDisabled } from "~/lib/errors";

import { buildPricing, type PlanCard } from "./plans";

function PlanCardView({ plan }: { plan: PlanCard }) {
	const titleId = `plan-${plan.id}`;
	return (
		<li
			aria-labelledby={titleId}
			className={`flex flex-col rounded-card border bg-surface p-6 shadow-sm ${
				plan.highlighted ? "border-brand-500 ring-1 ring-brand-500" : "border-line"
			}`}
		>
			<div className="flex items-center justify-between gap-2">
				<h3 id={titleId} className="text-lg font-semibold text-fg">
					{plan.name}
				</h3>
				{plan.highlighted ? <Badge tone="success">Popular</Badge> : null}
			</div>
			{plan.tagline ? (
				<p className="mt-1 text-sm text-fg-muted">{plan.tagline}</p>
			) : null}
			{plan.price ? (
				<p className="mt-4 flex items-baseline gap-1">
					<span className="text-3xl font-semibold text-fg">{plan.price}</span>
					{plan.period ? (
						<span className="text-sm text-fg-muted">{plan.period}</span>
					) : null}
				</p>
			) : null}
			{plan.bullets.length > 0 ? (
				<ul className="mt-4 flex-1 space-y-2 text-sm text-fg">
					{plan.bullets.map((bullet) => (
						<li key={bullet} className="flex gap-2">
							<span aria-hidden="true" className="text-success">
								&#10003;
							</span>
							{bullet}
						</li>
					))}
				</ul>
			) : (
				<div className="flex-1" />
			)}
			<Link
				to="/signup"
				onClick={() =>
					analytics.track("pricing_cta_clicked", {
						location: "landing",
						plan_id: plan.id,
					})
				}
				className={`${linkButtonClass(plan.highlighted ? "primary" : "secondary")} mt-6`}
			>
				{plan.ctaLabel}
			</Link>
		</li>
	);
}

export function PricingSection() {
	const prices = useQuery({ ...pricesQuery(), retry: false });
	const disabled = prices.isError && isBillingDisabled(prices.error);
	const pricing = buildPricing(prices.data ?? []);

	return (
		<section
			id="pricing"
			aria-labelledby="pricing-title"
			className="scroll-mt-16 border-b border-line bg-surface-muted"
		>
			<div className="mx-auto max-w-6xl px-4 py-16">
				<h2
					id="pricing-title"
					className="text-3xl font-semibold tracking-tight text-fg"
				>
					Pricing
				</h2>
				<p className="mt-2 max-w-2xl text-fg-muted">
					Start free. Upgrade a workspace when it needs more.
				</p>

				<ul className="mt-10 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
					{pricing.plans.map((plan) => (
						<PlanCardView key={plan.id} plan={plan} />
					))}
					{prices.isPending ? (
						<li aria-hidden="true">
							<Skeleton className="h-72 w-full" />
						</li>
					) : null}
				</ul>

				<div className="mt-6 space-y-3" aria-live="polite">
					{disabled ? (
						<p className="text-sm text-fg-muted">
							Paid plans are not enabled on this deployment, so only the Free
							plan is available.
						</p>
					) : null}
					{prices.isError && !disabled ? (
						<Alert tone="warning">
							We couldn't load the paid plans right now.{" "}
							<button
								type="button"
								className="font-medium underline"
								onClick={() => prices.refetch()}
							>
								Try again
							</button>
						</Alert>
					) : null}
				</div>

				{pricing.addOns.length > 0 ? (
					<div className="mt-10">
						<h3 className="text-lg font-semibold text-fg">Add-ons</h3>
						<ul className="mt-3 grid gap-3 sm:grid-cols-2">
							{pricing.addOns.map((addOn) => (
								<li
									key={addOn.id}
									className="rounded-card border border-line bg-surface p-4"
								>
									<p className="font-medium text-fg">{addOn.name}</p>
									{addOn.description ? (
										<p className="mt-1 text-sm text-fg-muted">
											{addOn.description}
										</p>
									) : null}
								</li>
							))}
						</ul>
					</div>
				) : null}
			</div>
		</section>
	);
}

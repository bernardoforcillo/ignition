import { Link } from "@tanstack/react-router";

import { linkButtonClass, PrimaryCta } from "~/features/site-chrome";

/** Decorative product sketch built from tokens only: no photos, nothing to download. */
function ProductSketch() {
	const rows = [
		["Ada Lovelace", "owner"],
		["Grace Hopper", "admin"],
		["Alan Turing", "member"],
	] as const;
	return (
		<div
			aria-hidden="true"
			className="mx-auto w-full max-w-md rounded-card border border-line bg-surface p-5 shadow-lg"
		>
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-2">
					<span className="size-6 rounded-md bg-brand-600" />
					<span className="text-sm font-semibold text-fg">Acme workspace</span>
				</div>
				<span className="rounded-full bg-success-soft px-2 py-0.5 text-xs font-medium text-success">
					Pro, active
				</span>
			</div>
			<ul className="mt-4 divide-y divide-line rounded-card border border-line">
				{rows.map(([name, role]) => (
					<li
						key={name}
						className="flex items-center justify-between px-3 py-2.5"
					>
						<span className="flex items-center gap-2 text-sm text-fg">
							<span className="size-6 rounded-full bg-brand-100" />
							{name}
						</span>
						<span className="rounded-full bg-surface-muted px-2 py-0.5 text-xs text-fg-muted">
							{role}
						</span>
					</li>
				))}
			</ul>
			<div className="mt-4">
				<div className="flex justify-between text-xs text-fg-muted">
					<span>API calls this month</span>
					<span>42,310 / 100,000</span>
				</div>
				<div className="mt-1.5 h-2 rounded-full bg-surface-muted">
					<div className="h-2 w-5/12 rounded-full bg-brand-500" />
				</div>
			</div>
		</div>
	);
}

export function Hero() {
	return (
		<section aria-labelledby="hero-title" className="border-b border-line">
			<div className="mx-auto grid max-w-6xl items-center gap-12 px-4 py-16 lg:grid-cols-2 lg:py-24">
				<div className="space-y-6">
					<p className="text-sm font-medium text-brand-600">
						An open SaaS starter
					</p>
					<h1
						id="hero-title"
						className="text-4xl font-semibold tracking-tight text-fg sm:text-5xl"
					>
						Launch your SaaS with the hard parts already built
					</h1>
					<p className="max-w-xl text-lg text-fg-muted">
						Accounts, workspaces with roles, plans and metered limits, Stripe
						billing and GDPR-ready data handling, wired end to end so you can
						spend your time on the product.
					</p>
					<div className="flex flex-wrap gap-3">
						<PrimaryCta label="Get started" />
						<Link
							to="/docs/$slug"
							params={{ slug: "overview" }}
							className={linkButtonClass("secondary")}
						>
							Read the docs
						</Link>
					</div>
				</div>
				<ProductSketch />
			</div>
		</section>
	);
}

import { Link } from "@tanstack/react-router";

import { ThemeToggle } from "~/features/theme-toggle";

import { SITE } from "./site-config";

const item =
	"rounded-card text-sm text-fg-muted hover:text-fg hover:underline focus-visible:outline-2 focus-visible:outline-brand-500";

export function SiteFooter() {
	return (
		<footer className="border-t border-line bg-surface-muted">
			<div className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-8 sm:flex-row sm:items-center sm:justify-between">
				<p className="text-sm text-fg-muted">
					{SITE.name}, a starter for SaaS products.
				</p>
				<nav aria-label="Footer">
					<ul className="flex flex-wrap gap-x-5 gap-y-2">
						<li>
							<Link to="/docs" className={item}>
								Docs
							</Link>
						</li>
						<li>
							<Link to="/login" className={item}>
								Sign in
							</Link>
						</li>
						<li>
							<Link
								to="/docs/$slug"
								params={{ slug: "privacy" }}
								className={item}
							>
								Privacy note
							</Link>
						</li>
						<li>
							<a
								href={SITE.githubUrl}
								className={item}
								rel="noopener noreferrer"
							>
								GitHub
							</a>
						</li>
					</ul>
				</nav>
				<ThemeToggle />
			</div>
		</footer>
	);
}

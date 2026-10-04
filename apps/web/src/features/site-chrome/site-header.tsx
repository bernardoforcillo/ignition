import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";

import { ThemeToggle } from "~/features/theme-toggle";
import { useAuthStore } from "~/stores/auth";

import { PrimaryCta } from "./primary-cta";
import { MAIN_CONTENT_ID, SITE } from "./site-config";

const navLink =
	"rounded-card px-2 py-1.5 text-sm font-medium text-fg-muted hover:text-fg focus-visible:outline-2 focus-visible:outline-brand-500";

type Props = {
	/** Extra control rendered before the nav, e.g. the docs menu button on phones. */
	leading?: ReactNode;
};

/** Skip link plus the banner landmark shared by the landing page and the docs. */
export function SiteHeader({ leading }: Props) {
	const signedIn = useAuthStore((state) => state.status === "authenticated");
	return (
		<>
			<a
				href={`#${MAIN_CONTENT_ID}`}
				className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-card focus:bg-surface focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-fg focus:shadow-lg focus:outline-2 focus:outline-brand-500"
			>
				Skip to content
			</a>
			<header className="sticky top-0 z-30 border-b border-line bg-surface/95 backdrop-blur">
				<div className="mx-auto flex h-14 max-w-6xl items-center gap-2 px-4">
					{leading}
					<Link
						to="/"
						className="mr-2 flex items-center gap-2 rounded-card text-base font-semibold text-fg focus-visible:outline-2 focus-visible:outline-brand-500"
					>
						<span
							aria-hidden="true"
							className="inline-block size-5 rounded-md bg-brand-600"
						/>
						{SITE.name}
					</Link>
					<nav aria-label="Main" className="ml-auto flex items-center gap-1">
						<Link to="/docs" className={navLink}>
							Docs
						</Link>
						<Link to="/" hash="pricing" className={navLink}>
							Pricing
						</Link>
						{signedIn ? null : (
							<Link to="/login" className={`${navLink} hidden sm:inline-block`}>
								Sign in
							</Link>
						)}
						<PrimaryCta className="ml-1" />
						<span className="ml-1 hidden sm:block">
							<ThemeToggle />
						</span>
					</nav>
				</div>
			</header>
		</>
	);
}

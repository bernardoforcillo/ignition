import { Link, Outlet } from "@tanstack/react-router";

import { ConsentBanner } from "~/features/consent";
import { useApplyTheme } from "~/features/theme-toggle";

export function RootLayout() {
	useApplyTheme();
	return (
		<div className="min-h-screen bg-surface text-fg">
			<Outlet />
			<ConsentBanner />
		</div>
	);
}

export function NotFoundPage() {
	return (
		<main className="mx-auto max-w-md space-y-3 px-4 py-24 text-center">
			<h1 className="text-xl font-semibold">Page not found</h1>
			<p className="text-sm text-fg-muted">
				The page you're looking for doesn't exist.
			</p>
			<Link to="/" className="font-medium text-brand-600 underline">
				Go home
			</Link>
		</main>
	);
}

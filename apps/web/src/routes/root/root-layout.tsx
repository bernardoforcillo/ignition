import { Link, Outlet } from "@tanstack/react-router";

import { useThemeStore } from "~/stores/theme";

export function RootLayout() {
	const theme = useThemeStore((state) => state.theme);
	const toggleTheme = useThemeStore((state) => state.toggleTheme);

	return (
		<div className="min-h-screen bg-white text-brand-900">
			<header className="flex items-center justify-between border-b border-brand-100 px-6 py-4">
				<nav className="flex gap-4 text-sm font-medium">
					<Link to="/" activeProps={{ className: "text-brand-600" }}>
						Home
					</Link>
					<Link to="/about" activeProps={{ className: "text-brand-600" }}>
						About
					</Link>
				</nav>
				<button
					type="button"
					onClick={toggleTheme}
					className="rounded-card border border-brand-100 px-3 py-1.5 text-xs font-medium hover:bg-brand-50"
				>
					Theme: {theme}
				</button>
			</header>

			<main className="mx-auto max-w-2xl space-y-6 px-6 py-8">
				<Outlet />
			</main>
		</div>
	);
}

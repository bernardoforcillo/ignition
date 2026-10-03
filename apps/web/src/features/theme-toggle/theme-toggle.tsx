import { useEffect } from "react";

import { useThemeStore } from "~/stores/theme";

/** Mirrors the persisted theme onto <html class="dark">, which the shared dark tokens key off. */
export function useApplyTheme(): void {
	const theme = useThemeStore((state) => state.theme);
	useEffect(() => {
		document.documentElement.classList.toggle("dark", theme === "dark");
	}, [theme]);
}

export function ThemeToggle() {
	const theme = useThemeStore((state) => state.theme);
	const toggleTheme = useThemeStore((state) => state.toggleTheme);
	return (
		<button
			type="button"
			onClick={toggleTheme}
			className="rounded-card border border-line px-3 py-1.5 text-xs font-medium text-fg hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500"
		>
			{theme === "dark" ? "Light theme" : "Dark theme"}
		</button>
	);
}

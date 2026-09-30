import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Theme = "light" | "dark";

interface ThemeState {
	theme: Theme;
	toggleTheme: () => void;
}

/**
 * A single, small domain slice for one real, intentional user preference (light/dark) — not a
 * catch-all store. Persisted because a theme choice is meant to survive a reload; an ephemeral
 * flag (a modal's open/closed state, for example) would stay local `useState` instead and never
 * reach a store at all.
 */
export const useThemeStore = create<ThemeState>()(
	persist(
		(set) => ({
			theme: "light",
			toggleTheme: () =>
				set((state) => ({ theme: state.theme === "light" ? "dark" : "light" })),
		}),
		{ name: "ignition-theme" },
	),
);

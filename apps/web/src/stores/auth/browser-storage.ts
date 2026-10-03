import type { KeyValueStorage } from "./auth-store";

/** localStorage that never throws (private windows, blocked site data): reads miss, writes drop. */
export const browserStorage: KeyValueStorage = {
	get(key) {
		try {
			return window.localStorage.getItem(key);
		} catch {
			return null;
		}
	},
	set(key, value) {
		try {
			window.localStorage.setItem(key, value);
		} catch {
			// Not persisted; the session still works until the page is closed.
		}
	},
	remove(key) {
		try {
			window.localStorage.removeItem(key);
		} catch {
			// Nothing to clean up.
		}
	},
};

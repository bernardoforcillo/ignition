import { Code, ConnectError } from "@connectrpc/connect";
import { create } from "zustand";

export interface AuthUser {
	id: string;
	email: string;
}

export interface Tokens {
	user: AuthUser;
	accessToken: string;
	refreshToken: string;
}

export interface AuthApi {
	login(email: string, password: string): Promise<Tokens>;
	refresh(refreshToken: string): Promise<Tokens>;
	logout(refreshToken: string): Promise<void>;
}

export interface KeyValueStorage {
	get(key: string): string | null;
	set(key: string, value: string): void;
	remove(key: string): void;
}

export interface AuthDeps {
	api: AuthApi;
	storage: KeyValueStorage;
	/** Runs after the session is cleared, e.g. to drop cached server data. */
	onCleared?: () => void;
}

export type AuthStatus = "unknown" | "authenticated" | "anonymous";

export interface AuthState {
	status: AuthStatus;
	user: AuthUser | null;
	accessToken: string | null;
	workspaceId: string | null;
	login(email: string, password: string): Promise<void>;
	/** Renews the access token. Concurrent callers share one request. Resolves `false` on failure. */
	refresh(): Promise<boolean>;
	/** Silent sign-in from the stored refresh token; runs once per page load. */
	restore(): Promise<void>;
	logout(): Promise<void>;
	setWorkspaceId(id: string | null): void;
	/** Forgets the session locally without calling the server (it is already gone, e.g. account deleted). */
	clearSession(): void;
}

export const REFRESH_TOKEN_KEY = "ignition-refresh-token";
export const WORKSPACE_KEY = "ignition-workspace-id";

/** The server refused the token itself (as opposed to a flaky network): the session is over. */
const isRejection = (err: unknown): boolean => {
	const { code } = ConnectError.from(err);
	return (
		code === Code.Unauthenticated ||
		code === Code.PermissionDenied ||
		code === Code.InvalidArgument ||
		code === Code.NotFound
	);
};

/**
 * Session state. Token storage trade-off: the ACCESS token lives in memory only (never reachable
 * after a reload, so XSS cannot read it from storage), while the REFRESH token sits in
 * localStorage so a reload can sign the user back in silently. localStorage is readable by any
 * script on the page; an httpOnly, SameSite cookie set by the server would be stronger, but the
 * API returns both tokens in the JSON body, so that is not available. The refresh token is
 * rotated on every use and revoked on logout, which bounds the damage of a leak.
 */
export const createAuthStore = ({ api, storage, onCleared }: AuthDeps) => {
	let inflightRefresh: Promise<boolean> | null = null;
	let restoring: Promise<void> | null = null;

	return create<AuthState>()((set, get) => {
		const apply = (tokens: Tokens) => {
			storage.set(REFRESH_TOKEN_KEY, tokens.refreshToken);
			set({
				status: "authenticated",
				user: tokens.user,
				accessToken: tokens.accessToken,
			});
		};

		const clear = () => {
			storage.remove(REFRESH_TOKEN_KEY);
			storage.remove(WORKSPACE_KEY);
			set({
				status: "anonymous",
				user: null,
				accessToken: null,
				workspaceId: null,
			});
			onCleared?.();
		};

		return {
			status: "unknown",
			user: null,
			accessToken: null,
			workspaceId: storage.get(WORKSPACE_KEY),

			async login(email, password) {
				apply(await api.login(email, password));
			},

			refresh() {
				inflightRefresh ??= (async () => {
					const token = storage.get(REFRESH_TOKEN_KEY);
					if (!token) {
						clear();
						return false;
					}
					try {
						apply(await api.refresh(token));
						return true;
					} catch (err) {
						if (isRejection(err)) clear();
						else set({ status: "anonymous", accessToken: null });
						return false;
					}
				})().finally(() => {
					inflightRefresh = null;
				});
				return inflightRefresh;
			},

			restore() {
				restoring ??= (async () => {
					if (!storage.get(REFRESH_TOKEN_KEY)) {
						set({ status: "anonymous" });
						return;
					}
					await get().refresh();
				})();
				return restoring;
			},

			async logout() {
				const token = storage.get(REFRESH_TOKEN_KEY);
				if (token) {
					try {
						await api.logout(token);
					} catch (err) {
						// Logout needs a live access token; renew once and retry with the rotated refresh token.
						if (ConnectError.from(err).code === Code.Unauthenticated) {
							const rotated = (await get().refresh())
								? storage.get(REFRESH_TOKEN_KEY)
								: null;
							if (rotated) await api.logout(rotated).catch(() => undefined);
						}
					}
				}
				clear();
			},

			clearSession: clear,

			setWorkspaceId(id) {
				if (id) storage.set(WORKSPACE_KEY, id);
				else storage.remove(WORKSPACE_KEY);
				set({ workspaceId: id });
			},
		};
	});
};

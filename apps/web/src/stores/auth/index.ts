import { queryClient } from "~/lib/query";
import { authClient, bindSession } from "~/lib/rpc";

import { type AuthApi, createAuthStore } from "./auth-store";
import { browserStorage } from "./browser-storage";

export type { AuthState, AuthStatus, AuthUser } from "./auth-store";

const api: AuthApi = {
	login: async (email, password) => {
		const res = await authClient.login({ email, password });
		return toTokens(res);
	},
	refresh: async (refreshToken) =>
		toTokens(await authClient.refresh({ refreshToken })),
	logout: async (refreshToken) => {
		await authClient.logout({ refreshToken });
	},
};

function toTokens(res: {
	user?: { id: string; email: string };
	accessToken: string;
	refreshToken: string;
}) {
	return {
		user: { id: res.user?.id ?? "", email: res.user?.email ?? "" },
		accessToken: res.accessToken,
		refreshToken: res.refreshToken,
	};
}

export const useAuthStore = createAuthStore({
	api,
	storage: browserStorage,
	onCleared: () => queryClient.clear(),
});

bindSession({
	getAccessToken: () => useAuthStore.getState().accessToken,
	refresh: () => useAuthStore.getState().refresh(),
});

/** Revokes the session, wipes cached data and returns to the sign-in screen's state. */
export const signOut = () => useAuthStore.getState().logout();

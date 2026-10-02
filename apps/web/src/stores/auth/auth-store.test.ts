import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";

import {
	type AuthApi,
	createAuthStore,
	type KeyValueStorage,
	REFRESH_TOKEN_KEY,
	type Tokens,
	WORKSPACE_KEY,
} from "./auth-store";

const tokens = (n: number): Tokens => ({
	user: { id: "u1", email: "ada@example.com" },
	accessToken: `access-${n}`,
	refreshToken: `refresh-${n}`,
});

const memoryStorage = (seed: Record<string, string> = {}) => {
	const data = new Map(Object.entries(seed));
	const storage: KeyValueStorage = {
		get: (k) => data.get(k) ?? null,
		set: (k, v) => void data.set(k, v),
		remove: (k) => void data.delete(k),
	};
	return { data, storage };
};

const setup = (
	seed: Record<string, string> = {},
	api: Partial<AuthApi> = {},
) => {
	const { data, storage } = memoryStorage(seed);
	const fullApi: AuthApi = {
		login: vi.fn(async () => tokens(1)),
		refresh: vi.fn(async () => tokens(2)),
		logout: vi.fn(async () => undefined),
		...api,
	};
	const onCleared = vi.fn();
	const store = createAuthStore({ api: fullApi, storage, onCleared });
	return { store, data, api: fullApi, onCleared };
};

describe("auth store", () => {
	it("starts unknown with the persisted workspace selection", () => {
		const { store } = setup({ [WORKSPACE_KEY]: "w1" });
		expect(store.getState()).toMatchObject({
			status: "unknown",
			accessToken: null,
			workspaceId: "w1",
		});
	});

	it("login keeps the access token in memory and persists only the refresh token", async () => {
		const { store, data } = setup();
		await store.getState().login("ada@example.com", "pw");

		expect(store.getState()).toMatchObject({
			status: "authenticated",
			accessToken: "access-1",
			user: { email: "ada@example.com" },
		});
		expect(data.get(REFRESH_TOKEN_KEY)).toBe("refresh-1");
		expect([...data.values()]).not.toContain("access-1");
	});

	it("a failed login leaves the state untouched", async () => {
		const { store } = setup(
			{},
			{
				login: vi.fn(async () => {
					throw new ConnectError("bad", Code.Unauthenticated);
				}),
			},
		);
		await expect(store.getState().login("a@b.co", "x")).rejects.toBeInstanceOf(
			ConnectError,
		);
		expect(store.getState().status).toBe("unknown");
	});

	it("restore signs in silently from the stored refresh token and rotates it", async () => {
		const { store, data, api } = setup({ [REFRESH_TOKEN_KEY]: "refresh-0" });
		await store.getState().restore();

		expect(api.refresh).toHaveBeenCalledWith("refresh-0");
		expect(store.getState().status).toBe("authenticated");
		expect(data.get(REFRESH_TOKEN_KEY)).toBe("refresh-2");
	});

	it("restore without a stored token becomes anonymous without calling the API", async () => {
		const { store, api } = setup();
		await store.getState().restore();
		expect(store.getState().status).toBe("anonymous");
		expect(api.refresh).not.toHaveBeenCalled();
	});

	it("restore runs once however many guards await it", async () => {
		const { store, api } = setup({ [REFRESH_TOKEN_KEY]: "refresh-0" });
		await Promise.all([store.getState().restore(), store.getState().restore()]);
		expect(api.refresh).toHaveBeenCalledOnce();
	});

	it("concurrent refreshes share one request", async () => {
		const { store, api } = setup({ [REFRESH_TOKEN_KEY]: "refresh-0" });
		const results = await Promise.all([
			store.getState().refresh(),
			store.getState().refresh(),
		]);
		expect(results).toEqual([true, true]);
		expect(api.refresh).toHaveBeenCalledOnce();
	});

	it("a rejected refresh token ends the session and clears storage", async () => {
		const { store, data, onCleared } = setup(
			{ [REFRESH_TOKEN_KEY]: "stale", [WORKSPACE_KEY]: "w1" },
			{
				refresh: vi.fn(async () => {
					throw new ConnectError("revoked", Code.Unauthenticated);
				}),
			},
		);
		expect(await store.getState().refresh()).toBe(false);

		expect(store.getState()).toMatchObject({
			status: "anonymous",
			accessToken: null,
		});
		expect(data.size).toBe(0);
		expect(onCleared).toHaveBeenCalled();
	});

	it("a network failure keeps the refresh token so the next load can retry", async () => {
		const { store, data } = setup(
			{ [REFRESH_TOKEN_KEY]: "refresh-0" },
			{
				refresh: vi.fn(async () => {
					throw new ConnectError("offline", Code.Unavailable);
				}),
			},
		);
		expect(await store.getState().refresh()).toBe(false);
		expect(data.get(REFRESH_TOKEN_KEY)).toBe("refresh-0");
	});

	it("logout revokes the refresh token server-side and clears everything", async () => {
		const { store, data, api, onCleared } = setup({ [WORKSPACE_KEY]: "w1" });
		await store.getState().login("ada@example.com", "pw");
		await store.getState().logout();

		expect(api.logout).toHaveBeenCalledWith("refresh-1");
		expect(store.getState()).toMatchObject({
			status: "anonymous",
			user: null,
			accessToken: null,
			workspaceId: null,
		});
		expect(data.size).toBe(0);
		expect(onCleared).toHaveBeenCalled();
	});

	it("logout still clears local state when the server call fails", async () => {
		const { store, data } = setup(
			{},
			{
				logout: vi.fn(async () => {
					throw new ConnectError("down", Code.Unavailable);
				}),
			},
		);
		await store.getState().login("ada@example.com", "pw");
		await store.getState().logout();
		expect(store.getState().status).toBe("anonymous");
		expect(data.size).toBe(0);
	});

	it("logout renews an expired session once and revokes the rotated refresh token", async () => {
		const logout = vi
			.fn<AuthApi["logout"]>()
			.mockRejectedValueOnce(new ConnectError("expired", Code.Unauthenticated))
			.mockResolvedValueOnce(undefined);
		const { store } = setup({}, { logout });
		await store.getState().login("ada@example.com", "pw");
		await store.getState().logout();

		expect(logout).toHaveBeenNthCalledWith(1, "refresh-1");
		expect(logout).toHaveBeenNthCalledWith(2, "refresh-2");
		expect(store.getState().status).toBe("anonymous");
	});

	it("persists and clears the selected workspace", () => {
		const { store, data } = setup();
		store.getState().setWorkspaceId("w9");
		expect(data.get(WORKSPACE_KEY)).toBe("w9");
		store.getState().setWorkspaceId(null);
		expect(data.has(WORKSPACE_KEY)).toBe(false);
	});
});

import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";

import { createAuthInterceptor, type Session } from "./auth-interceptor";

// A fake transport: records the Authorization header of every call and answers from a script.
const setup = (
	token: string | null,
	responses: Array<"ok" | Code>,
	service = "saas.v1.WorkspaceService",
) => {
	let current = token;
	const session: Session = {
		getAccessToken: () => current,
		refresh: vi.fn(async () => {
			current = "renewed";
			return true;
		}),
	};
	const seen: Array<string | null> = [];
	const script = [...responses];
	const next = vi.fn(async (req: { header: Headers }) => {
		seen.push(req.header.get("Authorization"));
		const step = script.shift() ?? "ok";
		if (step !== "ok") throw new ConnectError("nope", step);
		return { message: "done" };
	});
	const call = (stream = false) =>
		// biome-ignore lint/suspicious/noExplicitAny: minimal fake of Connect's request/handler types
		createAuthInterceptor(session)(next as any)({
			service: { typeName: service },
			method: { name: "Any" },
			header: new Headers(),
			stream,
			// biome-ignore lint/suspicious/noExplicitAny: see above
		} as any);
	return { session, seen, next, call };
};

describe("auth interceptor", () => {
	it("attaches the bearer token to authenticated calls", async () => {
		const { call, seen } = setup("abc", ["ok"]);
		await call();
		expect(seen).toEqual(["Bearer abc"]);
	});

	it("sends no Authorization header when signed out", async () => {
		const { call, seen } = setup(null, ["ok"]);
		await call();
		expect(seen).toEqual([null]);
	});

	it("refreshes once on unauthenticated and retries with the new token", async () => {
		const { call, seen, session } = setup("old", [Code.Unauthenticated, "ok"]);
		await expect(call()).resolves.toEqual({ message: "done" });
		expect(session.refresh).toHaveBeenCalledOnce();
		expect(seen).toEqual(["Bearer old", "Bearer renewed"]);
	});

	it("does not loop when the retry is unauthenticated again", async () => {
		const { call, next, session } = setup("old", [
			Code.Unauthenticated,
			Code.Unauthenticated,
		]);
		await expect(call()).rejects.toMatchObject({ code: Code.Unauthenticated });
		expect(session.refresh).toHaveBeenCalledOnce();
		expect(next).toHaveBeenCalledTimes(2);
	});

	it("surfaces the original error when refresh fails", async () => {
		const { call, session, next } = setup("old", [Code.Unauthenticated]);
		vi.mocked(session.refresh).mockResolvedValue(false);
		await expect(call()).rejects.toMatchObject({ code: Code.Unauthenticated });
		expect(next).toHaveBeenCalledOnce();
	});

	it("does not refresh for other error codes", async () => {
		const { call, session } = setup("old", [Code.PermissionDenied]);
		await expect(call()).rejects.toMatchObject({ code: Code.PermissionDenied });
		expect(session.refresh).not.toHaveBeenCalled();
	});

	it("does not refresh a call that carried no token (e.g. a wrong login)", async () => {
		const { call, session } = setup(null, [Code.Unauthenticated]);
		await expect(call()).rejects.toBeInstanceOf(ConnectError);
		expect(session.refresh).not.toHaveBeenCalled();
	});

	it("leaves AuthService calls alone", async () => {
		const { call, seen, session } = setup(
			"old",
			[Code.Unauthenticated],
			"saas.v1.AuthService",
		);
		await expect(call()).rejects.toBeInstanceOf(ConnectError);
		expect(seen).toEqual([null]);
		expect(session.refresh).not.toHaveBeenCalled();
	});
});

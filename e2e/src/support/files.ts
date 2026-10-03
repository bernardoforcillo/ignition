import type { APIRequestContext } from "@playwright/test";
import { expect } from "@playwright/test";

import { env } from "../harness/env";
import type { StoredObject } from "../harness/fake-s3";
import type { Account } from "./users";

export type { StoredObject };

/** What the fake bucket holds right now whose key lies under the given workspace. */
export async function storedObjects(
	workspaceId: string,
): Promise<StoredObject[]> {
	const all = (await (
		await fetch(`${env.storageUrl}/__objects`)
	).json()) as StoredObject[];
	return all.filter((o) => o.key.startsWith(`workspaces/${workspaceId}/`));
}

/** A bearer token for an account, for the RPCs a test calls directly (not through the UI). */
export async function accessToken(
	request: APIRequestContext,
	account: Account,
): Promise<string> {
	const res = await request.post("/saas.v1.AuthService/Login", {
		data: { email: account.email, password: account.password },
	});
	expect(res.ok()).toBe(true);
	return ((await res.json()) as { accessToken: string }).accessToken;
}

/**
 * Registers a pending file of the given size, which holds that many bytes of the workspace's quota
 * without uploading any (the signed URL is simply not used).
 */
export async function reserveStorage(
	request: APIRequestContext,
	token: string,
	workspaceId: string,
	sizeBytes: number,
): Promise<void> {
	const res = await request.post("/saas.v1.FileService/CreateUpload", {
		data: {
			workspaceId,
			name: "reservation.txt",
			contentType: "text/plain",
			sizeBytes: String(sizeBytes), // int64 travels as a string in Connect JSON
		},
		headers: { authorization: `Bearer ${token}` },
	});
	expect(res.ok()).toBe(true);
}

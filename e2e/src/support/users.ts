import { randomBytes } from "node:crypto";

import type {
	APIRequestContext,
	Browser,
	BrowserContext,
	Page,
} from "@playwright/test";
import { expect } from "@playwright/test";

import { tokenOf, waitForLink } from "./mail";

export const PASSWORD = "Correct-Horse-9-Battery!";

let counter = 0;

/** A unique, valid address; the counter plus a random suffix keeps parallel workers apart. */
export function uniqueEmail(label = "user"): string {
	counter += 1;
	return `${label}-${counter}-${randomBytes(4).toString("hex")}@e2e.test`;
}

/**
 * A unique client address for the gateway's per-IP rate limits: the preview proxy forwards this
 * header and the gateway trusts it from the loopback hop.
 */
function uniqueClientIp(): string {
	const [a, b, c] = randomBytes(3);
	return `10.${a}.${b}.${((c ?? 0) % 250) + 1}`;
}

/** A browser context with its own client IP (see above) and its own storage. */
export async function newContext(
	browser: Browser,
	baseURL: string,
): Promise<BrowserContext> {
	return browser.newContext({
		baseURL,
		acceptDownloads: true,
		// Animations are not under test and make "stable" checks slow.
		reducedMotion: "reduce",
		extraHTTPHeaders: { "x-forwarded-for": uniqueClientIp() },
	});
}

export interface Account {
	email: string;
	password: string;
}

const rpc = async (
	request: APIRequestContext,
	method: string,
	body: unknown,
	token?: string,
) =>
	request.post(`/saas.v1.${method}`, {
		data: body,
		headers: token ? { authorization: `Bearer ${token}` } : {},
	});

/**
 * Creates a verified account without driving the sign-up screens (those are covered by the
 * registration test): sign up, redeem the emailed token, done. `request` must come from a context
 * made by `newContext`, so it carries the context's client IP.
 */
export async function createVerifiedAccount(
	request: APIRequestContext,
	email = uniqueEmail(),
	password = PASSWORD,
): Promise<Account> {
	expect(
		(await rpc(request, "AuthService/SignUp", { email, password })).ok(),
	).toBe(true);
	const link = await waitForLink(email, "/verify-email");
	const verified = await rpc(request, "AuthService/VerifyEmail", {
		token: tokenOf(link),
	});
	expect(verified.ok()).toBe(true);
	return { email, password };
}

/** Signs in through the login screen. Lands wherever the app sends a signed-in user. */
export async function signIn(
	page: Page,
	account: Account,
	from = "/login",
): Promise<void> {
	await page.goto(from);
	await page.getByLabel("Email").fill(account.email);
	await page.getByLabel("Password", { exact: true }).fill(account.password);
	await page.getByRole("button", { name: "Sign in" }).click();
}

/** Onboarding screen: creates the first workspace and waits for the overview. */
export async function createWorkspace(page: Page, name: string): Promise<void> {
	await expect(page).toHaveURL(/\/onboarding$/);
	await page.getByLabel("Workspace name").fill(name);
	await page.getByRole("button", { name: "Create workspace" }).click();
	await expect(page).toHaveURL(/\/app$/);
	await expect(page.getByRole("heading", { name, level: 1 })).toBeVisible();
}

/** Workspace URLs are unique across the database: suffix the name so reruns and repeats never collide. */
export const uniqueName = (base: string): string =>
	`${base} ${randomBytes(3).toString("hex")}`;

/** A verified, signed-in user with a workspace, in the given (fresh) context. */
export async function signedInWithWorkspace(
	context: BrowserContext,
	workspaceName = uniqueName("Acme"),
) {
	const account = await createVerifiedAccount(context.request);
	const page = await context.newPage();
	await signIn(page, account);
	await createWorkspace(page, workspaceName);
	return { context, page, account, workspaceName };
}

/** The workspace id the app stored for the current session. */
export async function currentWorkspaceId(page: Page): Promise<string> {
	await expect
		.poll(() =>
			page.evaluate(() => localStorage.getItem("ignition-workspace-id")),
		)
		.toBeTruthy();
	return (await page.evaluate(() =>
		localStorage.getItem("ignition-workspace-id"),
	)) as string;
}

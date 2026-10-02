import { expect, test } from "../src/support/test";
import {
	createVerifiedAccount,
	newContext,
	signIn,
} from "../src/support/users";

test.describe("auth hygiene", () => {
	test("a signed-out visit to /app/... returns there after sign-in", async ({
		workspaceUser,
		isolated,
	}) => {
		const { account } = await workspaceUser("Return Trip");

		const page = await (await isolated()).newPage();
		await page.goto("/app/billing");
		await expect(page).toHaveURL(/\/login\?redirect=%2Fapp%2Fbilling$/);

		await page.getByLabel("Email").fill(account.email);
		await page.getByLabel("Password", { exact: true }).fill(account.password);
		await page.getByRole("button", { name: "Sign in" }).click();
		await expect(page).toHaveURL(/\/app\/billing$/);
		await expect(
			page.getByRole("heading", { name: "Billing", level: 1 }),
		).toBeVisible();
	});

	test("signing out revokes the session on the server, not just locally", async ({
		workspaceUser,
		isolated,
	}) => {
		const { page } = await workspaceUser("Sign Out Co");
		// Every refresh token that is live at some point around the sign-out: the stored one, plus any
		// that a refresh issued while signing out.
		const live = [
			await page.evaluate(() => localStorage.getItem("ignition-refresh-token")),
		];
		const logouts: number[] = [];
		page.on("response", async (response) => {
			if (response.url().endsWith("/saas.v1.AuthService/Logout")) {
				logouts.push(response.status());
				return;
			}
			if (!response.url().endsWith("/saas.v1.AuthService/Refresh")) return;
			const body = (await response.json().catch(() => null)) as {
				refreshToken?: string;
			} | null;
			if (body?.refreshToken) live.push(body.refreshToken);
		});

		await page.getByRole("button", { name: "Sign out" }).click();
		await expect(page).toHaveURL(/\/login$/);
		expect(
			await page.evaluate(() => localStorage.getItem("ignition-refresh-token")),
		).toBeNull();

		// The server accepted the Logout call itself (it needs the bearer token), first time.
		expect(logouts).toEqual([200]);

		// A copy of any of them (stolen, or kept by another tab) must no longer work.
		const other = (await isolated()).request;
		for (const refreshToken of live) {
			expect(refreshToken).toBeTruthy();
			const replay = await other.post("/saas.v1.AuthService/Refresh", {
				data: { refreshToken },
			});
			expect(replay.status()).toBe(401);
		}
	});

	test("/app signed out redirects to /login with the destination", async ({
		isolated,
	}) => {
		const page = await (await isolated()).newPage();
		await page.goto("/app");
		await expect(page).toHaveURL(/\/login\?redirect=%2Fapp$/);
		await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
	});

	for (const hostile of [
		"//evil.example",
		"/\\evil.example",
		"https://evil.example/phish",
		"javascript:alert(1)",
		"///evil.example",
	]) {
		test(`the redirect parameter ${JSON.stringify(hostile)} is not followed`, async ({
			browser,
			baseURL,
		}) => {
			const context = await newContext(browser, baseURL as string);
			const account = await createVerifiedAccount(context.request);
			const page = await context.newPage();
			const requested: string[] = [];
			page.on("request", (request) => requested.push(request.url()));

			await signIn(
				page,
				account,
				`/login?redirect=${encodeURIComponent(hostile)}`,
			);
			// Same origin, and the default landing for an account with no workspace yet.
			await expect(page).toHaveURL(/\/onboarding$/);
			expect(new URL(page.url()).origin).toBe(
				new URL(baseURL as string).origin,
			);
			expect(
				requested.filter((url) => new URL(url).hostname.includes("evil")),
			).toEqual([]);
			await context.close();
		});
	}
});

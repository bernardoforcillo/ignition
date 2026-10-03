import { expect, test } from "@playwright/test";

import { captures, resetCaptures, wire } from "../src/support/posthog";

// These tests share one fake PostHog host, so they must not overlap.
test.describe.configure({ mode: "serial" });

const SECRET = "tok_SECRET_0123456789abcdef";
const REDIRECT = "/app/billing?secret_redirect=1";

test.beforeEach(async ({ page }) => {
	await resetCaptures();
	// posthog-js treats an automated browser (navigator.webdriver) as a bot and drops its events,
	// which would make every "nothing leaks" assertion below vacuous.
	await page.addInitScript(() =>
		Object.defineProperty(navigator, "webdriver", { get: () => false }),
	);
});

/** Visits the screens whose URLs carry single-use secrets or a redirect target. */
async function visitSensitiveUrls(page: import("@playwright/test").Page) {
	await page.goto(`/reset-password?token=${SECRET}`);
	await expect(
		page.getByRole("heading", { name: "Reset password" }),
	).toBeVisible();
	await page.goto(`/verify-email?token=${SECRET}`);
	await expect(
		page.getByRole("heading", { name: /Email verif|Verifying/ }),
	).toBeVisible();
	await page.goto(`/login?redirect=${encodeURIComponent(REDIRECT)}`);
	await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
}

test("nothing is sent to PostHog before the visitor chooses", async ({
	page,
}) => {
	await visitSensitiveUrls(page);
	await expect(
		page.getByRole("region", { name: "Analytics consent" }),
	).toBeVisible();
	await page.waitForLoadState("networkidle");
	expect(await captures()).toEqual([]);
});

test("declining sends nothing, now or after a reload", async ({ page }) => {
	await page.goto("/login");
	await page.getByRole("button", { name: "Decline" }).click();
	await expect(
		page.getByRole("region", { name: "Analytics consent" }),
	).toHaveCount(0);

	await visitSensitiveUrls(page);
	await page.reload();
	await page.waitForLoadState("networkidle");
	await expect(
		page.getByRole("region", { name: "Analytics consent" }),
	).toHaveCount(0);
	expect(await captures()).toEqual([]);
});

test("after accepting, events flow but never carry tokens or redirect targets", async ({
	page,
}) => {
	await page.goto("/login");
	await page.getByRole("button", { name: "Accept analytics" }).click();
	await expect(
		page.getByRole("region", { name: "Analytics consent" }),
	).toHaveCount(0);

	await visitSensitiveUrls(page);
	// A marker page last: once its pageview has arrived, the earlier ones have too.
	await page.goto("/forgot-password?marker=done");
	await expect
		.poll(
			async () =>
				(await captures()).some((c) => c.body.includes("forgot-password")),
			{
				message: "a pageview for the last page reaches PostHog",
				timeout: 20_000,
			},
		)
		.toBe(true);

	const seen = await captures();
	// Positive control: pageviews for the sensitive routes were captured (with the secrets scrubbed).
	const bodies = seen.map((c) => c.body).join("\n");
	expect(bodies).toContain("$pageview");
	expect(bodies).toContain("/reset-password");

	for (const capture of seen) {
		const text = wire(capture);
		expect(text, `${capture.method} ${capture.url}`).not.toContain(SECRET);
		expect(text, `${capture.method} ${capture.url}`).not.toContain(
			"secret_redirect",
		);
	}
});

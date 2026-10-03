import type { Page } from "@playwright/test";

import { expect, test } from "../src/support/test";

/** Collects what the browser reports as an error, for the "clean console" assertions. */
function watchErrors(page: Page): string[] {
	const errors: string[] = [];
	page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
	page.on("console", (message) => {
		if (message.type() === "error") errors.push(`console: ${message.text()}`);
	});
	return errors;
}

test.describe("landing page", () => {
	test("shows the plans the gateway sells without signing in", async ({
		isolated,
	}) => {
		const page = await (await isolated()).newPage();
		const errors = watchErrors(page);
		// The price catalog is public: the call must not need (or carry) a token.
		const priceCalls: Array<string | undefined> = [];
		page.on("request", (request) => {
			if (request.url().includes("BillingService/ListPrices")) {
				priceCalls.push(request.headers().authorization);
			}
		});

		await page.goto("/");
		await expect(
			page.getByRole("heading", { level: 1, name: /launch your saas/i }),
		).toBeVisible();
		await expect(page).toHaveTitle(/Ignition/);

		const pricing = page.getByRole("region", { name: "Pricing" });
		await expect(
			pricing.getByRole("heading", { level: 3, name: "Free" }),
		).toBeVisible();
		// BILLING_PRICES in the e2e gateway: price_pro=plan:pro, price_extra=addon:extra-api-calls.
		await expect(
			pricing.getByRole("heading", { level: 3, name: "Pro" }),
		).toBeVisible();
		await expect(pricing.getByText("Extra API calls")).toBeVisible();
		expect(priceCalls.length).toBeGreaterThan(0);
		expect(priceCalls.every((header) => header === undefined)).toBe(true);

		// The FAQ is native <details>: closed until opened, no script needed.
		const faq = page.getByRole("region", {
			name: "Frequently asked questions",
		});
		await faq.getByText("What is Ignition?").click();
		await expect(faq.getByText(/React web app and a Go gateway/)).toBeVisible();

		expect(errors).toEqual([]);
	});

	test("Get started and the plan buttons lead to sign-up", async ({
		isolated,
	}) => {
		const page = await (await isolated()).newPage();
		await page.goto("/");
		await page
			.getByRole("banner")
			.getByRole("link", { name: "Get started" })
			.click();
		await expect(page).toHaveURL(/\/signup$/);

		await page.goto("/");
		await page.getByRole("link", { name: "Choose Pro" }).click();
		await expect(page).toHaveURL(/\/signup$/);
		await expect(
			page.getByRole("button", { name: "Create account" }),
		).toBeVisible();
	});

	test("a signed-in visitor sees Open app instead of Get started", async ({
		workspaceUser,
	}) => {
		const { page } = await workspaceUser("Landing Co");
		await page.goto("/");
		const header = page.getByRole("banner");
		await expect(header.getByRole("link", { name: "Open app" })).toBeVisible();
		await expect(page.getByRole("link", { name: "Get started" })).toHaveCount(
			0,
		);
		await header.getByRole("link", { name: "Open app" }).click();
		await expect(page).toHaveURL(/\/app$/);
	});

	test("has a skip link, landmarks and a working phone layout", async ({
		browser,
		baseURL,
	}) => {
		const context = await browser.newContext({
			baseURL: baseURL as string,
			viewport: { width: 375, height: 700 },
		});
		const page = await context.newPage();
		await page.goto("/");
		await expect(page.getByRole("main")).toBeVisible();
		await expect(page.getByRole("contentinfo")).toBeVisible();
		await page.keyboard.press("Tab");
		await expect(
			page.getByRole("link", { name: "Skip to content" }),
		).toBeFocused();
		// No horizontal page scroll at phone width.
		const overflow = await page.evaluate(
			() => document.documentElement.scrollWidth - window.innerWidth,
		);
		expect(overflow).toBeLessThanOrEqual(0);
		await context.close();
	});
});

test.describe("documentation", () => {
	test("sidebar, next link and in-page anchors", async ({ isolated }) => {
		const page = await (await isolated()).newPage();
		const errors = watchErrors(page);

		await page.goto("/docs");
		await expect(page).toHaveURL(/\/docs\/overview$/);
		await expect(
			page.getByRole("heading", { level: 1, name: "Overview" }),
		).toBeVisible();

		const sidebar = page.getByRole("navigation", { name: "Documentation" });
		await expect(
			sidebar.getByRole("link", { name: "Overview" }),
		).toHaveAttribute("aria-current", "page");
		await sidebar
			.getByRole("link", { name: "Authentication and security" })
			.click();
		await expect(page).toHaveURL(/\/docs\/authentication$/);
		await expect(
			page.getByRole("heading", {
				level: 1,
				name: "Authentication and security",
			}),
		).toBeVisible();
		await expect(page).toHaveTitle(/Authentication and security/);
		await expect(
			sidebar.getByRole("link", { name: "Authentication and security" }),
		).toHaveAttribute("aria-current", "page");

		// An in-page anchor scrolls to its heading and is part of the URL.
		await page
			.getByRole("navigation", { name: "On this page" })
			.getByRole("link", { name: "Rate limits" })
			.click();
		await expect(page).toHaveURL(/\/docs\/authentication#rate-limits$/);
		await expect(
			page.getByRole("heading", { level: 2, name: "Rate limits" }),
		).toBeInViewport();

		// Prev/next follow the manifest order.
		const pager = page.getByRole("navigation", { name: "Pagination" });
		await expect(
			pager.getByRole("link", { name: /Previous\s*Getting started/ }),
		).toBeVisible();
		await pager
			.getByRole("link", { name: /Next\s*Workspaces and roles/ })
			.click();
		await expect(page).toHaveURL(/\/docs\/workspaces$/);
		await expect(
			page.getByRole("heading", { level: 1, name: "Workspaces and roles" }),
		).toBeVisible();

		// A cross-page link written in Markdown is a client-side navigation.
		await page.goto("/docs/getting-started");
		await page.getByRole("link", { name: "password policy" }).click();
		await expect(page).toHaveURL(/\/docs\/authentication#password-policy$/);

		expect(errors).toEqual([]);
	});

	test("code blocks have a copy button; an unknown page says so", async ({
		isolated,
	}) => {
		const context = await isolated();
		await context.grantPermissions(["clipboard-read", "clipboard-write"]);
		const page = await context.newPage();
		await page.goto("/docs/api");
		const copy = page
			.getByRole("button", { name: /Copy code to clipboard/ })
			.first();
		await copy.click();
		await expect(
			page.getByRole("button", { name: /Copied/ }).first(),
		).toBeVisible();
		await expect
			.poll(() => page.evaluate(() => navigator.clipboard.readText()))
			.toContain("BASE=");

		await page.goto("/docs/nope");
		await expect(
			page.getByRole("heading", { name: "Page not found" }),
		).toBeVisible();
	});

	test("the phone menu opens a drawer that closes after navigating", async ({
		browser,
		baseURL,
	}) => {
		const context = await browser.newContext({
			baseURL: baseURL as string,
			viewport: { width: 375, height: 700 },
		});
		const page = await context.newPage();
		await page.goto("/docs/overview");
		const sidebar = page.getByRole("navigation", { name: "Documentation" });
		await expect(sidebar).toBeHidden();
		await page.getByRole("button", { name: "Menu" }).click();
		await expect(sidebar).toBeVisible();
		await sidebar.getByRole("link", { name: "Your data" }).click();
		await expect(page).toHaveURL(/\/docs\/your-data$/);
		await expect(sidebar).toBeHidden();
		await context.close();
	});
});

test("robots.txt and sitemap.xml are served", async ({ isolated }) => {
	const request = (await isolated()).request;
	const robots = await request.get("/robots.txt");
	expect(robots.ok()).toBe(true);
	expect(await robots.text()).toContain("Sitemap:");
	const sitemap = await request.get("/sitemap.xml");
	expect(sitemap.ok()).toBe(true);
	expect(await sitemap.text()).toContain("/docs/overview");
});

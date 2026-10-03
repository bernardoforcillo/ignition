import { existsSync } from "node:fs";
import path from "node:path";

import { chromium, defineConfig } from "@playwright/test";

import { freePort } from "./src/harness/ports";

// The web ports must be known here (they are the projects' baseURL) and by global setup, which runs
// in this same process; workers re-evaluate this file with the inherited environment.
process.env.E2E_WEB_PORT ??= String(await freePort());
process.env.E2E_WEB_ANALYTICS_PORT ??= String(await freePort());

/** Prefer the bundled browser; fall back to a pre-installed one (CI image, sandbox). */
function chromiumPath(): string | undefined {
	if (process.env.E2E_CHROMIUM_PATH) return process.env.E2E_CHROMIUM_PATH;
	if (existsSync(chromium.executablePath())) return undefined;
	const preinstalled = path.join(
		process.env.PLAYWRIGHT_BROWSERS_PATH ?? "",
		"chromium",
	);
	return existsSync(preinstalled) ? preinstalled : undefined;
}

const ci = Boolean(process.env.CI);
const executablePath = chromiumPath();

export default defineConfig({
	testDir: "./tests",
	globalSetup: "./src/harness/global-setup.ts",
	fullyParallel: true,
	forbidOnly: ci,
	retries: ci ? 1 : 0,
	// Tests own their data, so any worker count is valid; keep at least two locally to prove it.
	workers: process.env.E2E_WORKERS
		? Number(process.env.E2E_WORKERS)
		: ci
			? 2
			: 3,
	reporter: ci ? [["github"], ["html", { open: "never" }]] : [["list"]],
	timeout: 60_000,
	expect: { timeout: 10_000 },
	use: {
		trace: "on-first-retry",
		launchOptions: executablePath ? { executablePath } : {},
		acceptDownloads: true,
	},
	projects: [
		{
			name: "chromium",
			testIgnore: "analytics*.spec.ts",
			use: { baseURL: `http://127.0.0.1:${process.env.E2E_WEB_PORT}` },
		},
		{
			// The same browser against the web build that has PostHog configured.
			name: "analytics",
			testMatch: "analytics*.spec.ts",
			use: {
				baseURL: `http://127.0.0.1:${process.env.E2E_WEB_ANALYTICS_PORT}`,
				// posthog-js drops events from user agents that look like bots, and headless Chromium
				// announces itself as "HeadlessChrome": present an ordinary desktop one.
				userAgent:
					"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
			},
		},
	],
});

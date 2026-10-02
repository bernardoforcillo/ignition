import path from "node:path";
import { fileURLToPath } from "node:url";

import { DEFAULT_DATABASE_URL, resetDatabase } from "./database.js";
import {
	ADDON_PRICE_ID,
	PLAN_PRICE_ID,
	POSTHOG_KEY,
	RESEND_KEY,
	STRIPE_KEY,
	WEBHOOK_SECRET,
} from "./env.js";
import { startFakePostHog } from "./fake-posthog.js";
import { startFakeResend } from "./fake-resend.js";
import { startFakeStripe } from "./fake-stripe.js";
import { freePort } from "./ports.js";
import { type Managed, run, start, waitForHttp } from "./process.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const e2eDir = path.resolve(here, "../..");
const repoDir = path.resolve(e2eDir, "..");
const runDir = path.join(e2eDir, ".run");

/**
 * Brings up the whole stack, in dependency order, and returns the teardown Playwright calls at the
 * end: fake Resend, fake Stripe, fake PostHog, a fresh Postgres database, the gateway (a binary built
 * from apps/gateway), then two builds of apps/web served by `vite preview` (one plain, one with
 * analytics configured) that proxy the API to the gateway, so the browser sees one origin.
 */
export default async function globalSetup(): Promise<() => Promise<void>> {
	const stops: Array<() => Promise<void>> = [];
	const teardown = async () => {
		for (const stop of stops.reverse()) await stop().catch(() => undefined);
	};
	try {
		await boot(stops);
	} catch (error) {
		await teardown();
		throw error;
	}
	return teardown;
}

async function boot(stops: Array<() => Promise<void>>): Promise<void> {
	const databaseUrl = process.env.E2E_DATABASE_URL ?? DEFAULT_DATABASE_URL;
	const webPort = Number(process.env.E2E_WEB_PORT);
	const webAnalyticsPort = Number(process.env.E2E_WEB_ANALYTICS_PORT);
	const webUrl = `http://127.0.0.1:${webPort}`;
	const webAnalyticsUrl = `http://127.0.0.1:${webAnalyticsPort}`;

	resetDatabase(databaseUrl);

	const resend = await startFakeResend(RESEND_KEY);
	const stripe = await startFakeStripe(STRIPE_KEY);
	const posthog = await startFakePostHog();
	stops.push(resend.close, stripe.close, posthog.close);

	// The gateway: built once (so a compile error fails fast with its output), then run.
	const gatewayBin = path.join(runDir, "gateway");
	await run("go", ["build", "-o", gatewayBin, "."], {
		cwd: path.join(repoDir, "apps/gateway"),
		log: path.join(runDir, "gateway-build.log"),
	});
	const gatewayPort = await freePort();
	const gatewayUrl = `http://127.0.0.1:${gatewayPort}`;
	const gateway = start(gatewayBin, [], {
		cwd: path.join(repoDir, "apps/gateway"),
		log: path.join(runDir, "gateway.log"),
		env: {
			...process.env,
			GATEWAY_LISTEN_ADDR: `127.0.0.1:${gatewayPort}`,
			// A route is required to start; nothing in the suite proxies to it.
			GATEWAY_UPSTREAM_URL: "http://127.0.0.1:1",
			DATABASE_URL: databaseUrl,
			AUTH_SECRET: "e2e-auth-secret-at-least-32-bytes-long!!",
			APP_URL: webUrl,
			COMPANY_NAME: "Ignition E2E",
			MAIL_FROM: "Ignition E2E <hello@e2e.test>",
			RESEND_API_KEY: RESEND_KEY,
			RESEND_BASE_URL: resend.url,
			STRIPE_API_KEY: STRIPE_KEY,
			STRIPE_API_BASE_URL: stripe.url,
			STRIPE_WEBHOOK_SECRET: WEBHOOK_SECRET,
			BILLING_PRICES: `${PLAN_PRICE_ID}=plan:pro,${ADDON_PRICE_ID}=addon:extra-api-calls`,
			// Every test runs from 127.0.0.1, which would exhaust the per-IP sign-up (10/h) and login
			// budgets across the suite. The preview proxy is a trusted hop, so each browser context
			// presents its own X-Forwarded-For address and gets its own budget.
			TRUSTED_PROXIES: "127.0.0.1/32,::1/128",
			LOG_FORMAT: "json",
			RATE_LIMIT_RPS: "",
			GATEWAY_AUTH_TOKEN: "",
			POSTHOG_API_KEY: "",
		},
	});
	stops.push(gateway.stop);
	await waitForHttp(`${gatewayUrl}/readyz`, 60_000, () => alive(gateway));

	// Two builds of the web app: VITE_* is baked in at build time.
	const webEnv = (extra: NodeJS.ProcessEnv): NodeJS.ProcessEnv => {
		const base = { ...process.env, ...extra };
		delete base.VITE_API_URL; // unset means "the page origin", which the preview proxies
		return base;
	};
	const build = (name: string, extra: NodeJS.ProcessEnv) =>
		run(
			"pnpm",
			[
				"--filter",
				"@ignition/web",
				"exec",
				"vite",
				"build",
				"--outDir",
				path.join(runDir, name),
				"--emptyOutDir",
			],
			{
				cwd: repoDir,
				env: webEnv(extra),
				log: path.join(runDir, `web-build-${name}.log`),
			},
		);
	await Promise.all([
		build("web", { VITE_POSTHOG_KEY: "", VITE_POSTHOG_HOST: "" }),
		build("web-analytics", {
			VITE_POSTHOG_KEY: POSTHOG_KEY,
			VITE_POSTHOG_HOST: posthog.url,
		}),
	]);
	const preview = async (name: string, port: number) => {
		const server: Managed = start(
			"pnpm",
			[
				"--filter",
				"@ignition/web",
				"exec",
				"vite",
				"preview",
				"--outDir",
				path.join(runDir, name),
				"--host",
				"127.0.0.1",
				"--port",
				String(port),
				"--strictPort",
			],
			{
				cwd: repoDir,
				env: webEnv({ VITE_API_PROXY_TARGET: gatewayUrl }),
				log: path.join(runDir, `web-preview-${name}.log`),
			},
		);
		stops.push(server.stop);
		await waitForHttp(`http://127.0.0.1:${port}/`, 60_000, () => alive(server));
	};
	await Promise.all([
		preview("web", webPort),
		preview("web-analytics", webAnalyticsPort),
	]);

	Object.assign(process.env, {
		E2E_GATEWAY_URL: gatewayUrl,
		E2E_RESEND_URL: resend.url,
		E2E_STRIPE_URL: stripe.url,
		E2E_POSTHOG_URL: posthog.url,
		E2E_WEB_URL: webUrl,
		E2E_WEB_ANALYTICS_URL: webAnalyticsUrl,
	});
}

const alive = (managed: Managed): boolean =>
	managed.child.exitCode === null && managed.child.signalCode === null;

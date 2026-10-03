import type { PostHog } from "posthog-js";

import type { AnalyticsClient } from "./analytics";
import { scrubProperties } from "./scrub";

export interface AnalyticsConfig {
	/** PostHog project API key (write-only; safe to ship to the browser). */
	apiKey: string;
	/** Ingestion host; defaults to the EU region. */
	host: string;
}

export const DEFAULT_POSTHOG_HOST = "https://eu.i.posthog.com";

/** Reads the config from Vite env; null (analytics off) when no key is set. */
export function readAnalyticsConfig(env: {
	VITE_POSTHOG_KEY?: string;
	VITE_POSTHOG_HOST?: string;
}): AnalyticsConfig | null {
	const apiKey = env.VITE_POSTHOG_KEY?.trim();
	if (!apiKey) return null;
	return {
		apiKey,
		host: env.VITE_POSTHOG_HOST?.trim() || DEFAULT_POSTHOG_HOST,
	};
}

/**
 * Wraps posthog-js for an EU product:
 *  - the SDK is not even initialised until the visitor accepts: `init` fetches the vendor's config
 *    and scripts and posts to its feature-flag endpoint (with an anonymous id and the visitor's IP),
 *    which is data leaving the browser before any consent. Until then every call is a no-op;
 *  - once started it is still opted out by default (`opt_out_capturing_by_default`) until
 *    `optIn` runs, which is the same call that starts it;
 *  - person profiles only for identified (signed-in) users;
 *  - pageviews are sent by the router bridge, not automatically, so the URL can be scrubbed;
 *  - every event passes through `before_send`, which strips tokens and redirect targets from URLs
 *    (the reset, verification and invitation links carry single-use secrets in the query string);
 *  - session replay masks every input; autocapture records clicks and submits only, never copied
 *    text.
 */
export function createPostHogClient(config: AnalyticsConfig): AnalyticsClient {
	let started = false;
	let loaded: PostHog | null = null;
	// Calls made while the SDK is still downloading (it is a separate chunk, fetched only after
	// consent, so visitors who decline never download it) run, in order, once it is ready.
	const waiting: Array<(posthog: PostHog) => void> = [];

	const start = () => {
		if (started) return;
		started = true;
		void import("posthog-js").then(({ default: posthog }) => {
			initPostHog(posthog, config);
			loaded = posthog;
			for (const call of waiting.splice(0)) call(posthog);
		});
	};
	/** Runs `call` now if the SDK is ready, later if it is loading, never if consent was not given. */
	const run = (call: (posthog: PostHog) => void) => {
		if (loaded) call(loaded);
		else if (started) waiting.push(call);
	};

	return {
		capture: (event, properties) =>
			run((posthog) => posthog.capture(event, properties)),
		captureException: (error, properties) =>
			run((posthog) => posthog.captureException(error, properties)),
		identify: (distinctId) => run((posthog) => posthog.identify(distinctId)),
		reset: () => run((posthog) => posthog.reset()),
		optIn: () => {
			start();
			run((posthog) => posthog.opt_in_capturing());
		},
		optOut: () => run((posthog) => posthog.opt_out_capturing()),
		isFeatureEnabled: (key) => loaded?.isFeatureEnabled(key),
		onFeatureFlags: (callback) => {
			let unsubscribe: (() => void) | undefined;
			run((posthog) => {
				unsubscribe = posthog.onFeatureFlags(callback);
			});
			return () => unsubscribe?.();
		},
	};
}

function initPostHog(posthog: PostHog, config: AnalyticsConfig): void {
	posthog.init(config.apiKey, {
		api_host: config.host,
		opt_out_capturing_by_default: true,
		person_profiles: "identified_only",
		capture_pageview: false,
		capture_pageleave: true,
		capture_exceptions: true,
		respect_dnt: true,
		autocapture: {
			dom_event_allowlist: ["click", "submit"],
			capture_copied_text: false,
		},
		session_recording: { maskAllInputs: true },
		before_send: (event) => {
			if (event) event.properties = scrubProperties(event.properties) ?? {};
			return event;
		},
	});
}

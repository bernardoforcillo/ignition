import posthog from "posthog-js";

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
	const start = () => {
		if (started) return;
		started = true;
		initPostHog(config);
	};

	return {
		capture: (event, properties) => {
			if (started) posthog.capture(event, properties);
		},
		captureException: (error, properties) => {
			if (started) posthog.captureException(error, properties);
		},
		identify: (distinctId) => {
			if (started) posthog.identify(distinctId);
		},
		reset: () => {
			if (started) posthog.reset();
		},
		optIn: () => {
			start();
			posthog.opt_in_capturing();
		},
		optOut: () => {
			if (started) posthog.opt_out_capturing();
		},
		isFeatureEnabled: (key) =>
			started ? posthog.isFeatureEnabled(key) : undefined,
		onFeatureFlags: (callback) =>
			started ? posthog.onFeatureFlags(callback) : () => {},
	};
}

function initPostHog(config: AnalyticsConfig): void {
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

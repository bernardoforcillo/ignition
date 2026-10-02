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
 * Initialises posthog-js for an EU product:
 *  - opted out until the visitor accepts (`opt_out_capturing_by_default`), so nothing is stored or
 *    sent before consent;
 *  - person profiles only for identified (signed-in) users;
 *  - pageviews are sent by the router bridge, not automatically, so the URL can be scrubbed;
 *  - every event passes through `before_send`, which strips tokens and redirect targets from URLs
 *    (the reset, verification and invitation links carry single-use secrets in the query string);
 *  - session replay masks every input; autocapture records clicks and submits only, never copied
 *    text.
 */
export function createPostHogClient(config: AnalyticsConfig): AnalyticsClient {
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

	return {
		capture: (event, properties) => posthog.capture(event, properties),
		captureException: (error, properties) =>
			posthog.captureException(error, properties),
		identify: (distinctId) => posthog.identify(distinctId),
		reset: () => posthog.reset(),
		optIn: () => posthog.opt_in_capturing(),
		optOut: () => posthog.opt_out_capturing(),
		isFeatureEnabled: (key) => posthog.isFeatureEnabled(key),
		onFeatureFlags: (callback) => posthog.onFeatureFlags(callback),
	};
}

import { type Analytics, createAnalytics } from "./analytics";
import { createPostHogClient, readAnalyticsConfig } from "./posthog-client";

export type {
	Analytics,
	AnalyticsClient,
	ConsentDecision,
} from "./analytics";
export { createAnalytics } from "./analytics";
export type { AnalyticsEventName, AnalyticsEvents } from "./events";
export { identityAction } from "./identity";
export { scrubProperties, scrubUrl } from "./scrub";

const config = readAnalyticsConfig(import.meta.env);

/**
 * The app's analytics: PostHog when `VITE_POSTHOG_KEY` is set, a no-op otherwise (local
 * development, tests, forks that do not use PostHog). Import this; never `posthog-js` directly.
 */
export const analytics: Analytics = createAnalytics(
	config ? createPostHogClient(config) : null,
);

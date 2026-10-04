import type { AnalyticsEventName, AnalyticsEvents } from "./events";

/** The slice of the vendor SDK the app uses. The real one is posthog-js; tests pass a fake. */
export interface AnalyticsClient {
	capture(event: string, properties?: Record<string, unknown>): void;
	captureException(error: unknown, properties?: Record<string, unknown>): void;
	identify(distinctId: string): void;
	reset(): void;
	optIn(): void;
	optOut(): void;
	isFeatureEnabled(key: string): boolean | undefined;
	onFeatureFlags(callback: () => void): () => void;
}

export type ConsentDecision = "unset" | "granted" | "denied";

export interface Analytics {
	/** False when no vendor key is configured: every method below is then a no-op. */
	readonly enabled: boolean;
	track<N extends AnalyticsEventName>(
		event: N,
		properties: AnalyticsEvents[N],
	): void;
	pageview(path: string): void;
	identify(accountId: string): void;
	reset(): void;
	captureException(error: unknown, properties?: Record<string, unknown>): void;
	applyConsent(decision: ConsentDecision): void;
	isFeatureEnabled(key: string): boolean;
	onFeatureFlags(callback: () => void): () => void;
}

/**
 * Wraps a client (or null when analytics is off) behind the app's typed surface. Capturing before
 * the visitor consents is safe: the SDK is initialised opted out and drops those events, so callers
 * never check consent themselves.
 */
export function createAnalytics(client: AnalyticsClient | null): Analytics {
	return {
		enabled: client !== null,
		track: (event, properties) => client?.capture(event, properties),
		pageview: (path) => client?.capture("$pageview", { $pathname: path }),
		identify: (accountId) => {
			// An empty id would merge every anonymous visitor into one person.
			if (accountId) client?.identify(accountId);
		},
		reset: () => client?.reset(),
		captureException: (error, properties) =>
			client?.captureException(error, properties),
		applyConsent: (decision) => {
			if (decision === "granted") client?.optIn();
			else client?.optOut();
		},
		isFeatureEnabled: (key) => client?.isFeatureEnabled(key) === true,
		onFeatureFlags: (callback) =>
			client?.onFeatureFlags(callback) ?? (() => {}),
	};
}

/// <reference types="vite/client" />

interface ImportMetaEnv {
	/** Gateway origin for the browser; unset, requests go to the page's own origin. */
	readonly VITE_API_URL?: string;
	/** PostHog project API key; unset disables analytics entirely. */
	readonly VITE_POSTHOG_KEY?: string;
	/** PostHog ingestion host, default https://eu.i.posthog.com (EU region). */
	readonly VITE_POSTHOG_HOST?: string;
}

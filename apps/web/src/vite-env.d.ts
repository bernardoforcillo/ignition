/// <reference types="vite/client" />

interface ImportMetaEnv {
	/** Gateway origin for the browser; unset, requests go to the page's own origin. */
	readonly VITE_API_URL?: string;
}

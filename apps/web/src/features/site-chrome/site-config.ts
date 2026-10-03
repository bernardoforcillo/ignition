/** Shared by every public page (landing, docs). */
export const SITE = {
	name: "Ignition",
	/** PLACEHOLDER: point this at the repository of your fork. */
	githubUrl: "https://github.com/OWNER/ignition",
} as const;

/** The id the skip link jumps to; each public page puts it on its <main>. */
export const MAIN_CONTENT_ID = "main-content";

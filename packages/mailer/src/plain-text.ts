import { plainTextSelectors } from "@react-email/render";

/**
 * Options for the plain-text alternative. The defaults upper-case headings, which would also
 * upper-case the `{{.Variable}}` placeholders the Go export puts in them (`{{.WORKSPACENAME}}`).
 */
export const plainTextOptions = {
	plainText: true,
	htmlToTextOptions: {
		selectors: [
			...plainTextSelectors,
			{ selector: "h1", options: { uppercase: false } },
			{ selector: "h2", options: { uppercase: false } },
			{ selector: "h3", options: { uppercase: false } },
		],
	},
} as const;

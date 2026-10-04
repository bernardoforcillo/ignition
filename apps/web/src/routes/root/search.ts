/** Query-string values are untrusted: keep strings only. */
export const stringParam = (value: unknown): string | undefined =>
	typeof value === "string" && value.length > 0 ? value : undefined;

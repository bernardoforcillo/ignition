const MAX_LENGTH = 40;

/** "Acme Inc.!" -> "acme-inc": lowercase ASCII words joined by single hyphens. */
export function slugify(name: string): string {
	return name
		.normalize("NFKD")
		.replace(/[̀-ͯ]/g, "")
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "")
		.slice(0, MAX_LENGTH)
		.replace(/-+$/, "");
}

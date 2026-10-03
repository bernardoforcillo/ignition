type Variant = "primary" | "secondary";

const base =
	"inline-flex items-center justify-center rounded-card px-4 py-2 text-sm font-medium shadow-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500";

const variants: Record<Variant, string> = {
	primary: "bg-brand-600 text-white hover:bg-brand-700",
	secondary: "border border-line bg-surface text-fg hover:bg-surface-muted",
};

/** Classes that make a router <Link> look like the shared Button. */
export const linkButtonClass = (variant: Variant = "primary"): string =>
	`${base} ${variants[variant]}`;

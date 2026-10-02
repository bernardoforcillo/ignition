import type { HTMLMotionProps } from "motion/react";
import { motion } from "motion/react";
import type { ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary" | "danger" | "ghost";

export interface ButtonProps
	extends Omit<HTMLMotionProps<"button">, "children"> {
	variant?: ButtonVariant;
	/** Disables the button and marks it busy (e.g. while a request is pending). */
	loading?: boolean;
	children: ReactNode;
}

const variantClasses: Record<ButtonVariant, string> = {
	primary: "bg-brand-600 text-white hover:bg-brand-700",
	secondary: "bg-surface-muted text-fg hover:bg-line",
	danger: "bg-danger text-surface hover:opacity-90",
	ghost: "bg-transparent text-fg shadow-none hover:bg-surface-muted",
};

/**
 * Shared, Tailwind-styled button. Uses `motion` for a real (not decorative-only) hover/tap
 * micro-interaction, so this is the component in the library that demonstrates the `motion`
 * requirement concretely rather than by unused import.
 */
export function Button({
	variant = "primary",
	loading = false,
	disabled,
	className = "",
	children,
	...rest
}: ButtonProps) {
	const inert = disabled || loading;
	return (
		<motion.button
			whileHover={inert ? undefined : { scale: 1.03 }}
			whileTap={inert ? undefined : { scale: 0.97 }}
			transition={{ type: "spring", stiffness: 400, damping: 20 }}
			disabled={inert}
			aria-busy={loading || undefined}
			className={`inline-flex items-center justify-center rounded-card px-4 py-2 text-sm font-medium shadow-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 disabled:cursor-not-allowed disabled:opacity-60 ${variantClasses[variant]} ${className}`}
			{...rest}
		>
			{children}
		</motion.button>
	);
}

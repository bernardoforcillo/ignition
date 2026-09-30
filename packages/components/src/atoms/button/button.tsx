import type { HTMLMotionProps } from "motion/react";
import { motion } from "motion/react";
import type { ReactNode } from "react";

export type ButtonVariant = "primary" | "secondary";

export interface ButtonProps
	extends Omit<HTMLMotionProps<"button">, "children"> {
	variant?: ButtonVariant;
	children: ReactNode;
}

const variantClasses: Record<ButtonVariant, string> = {
	primary: "bg-brand-600 text-white hover:bg-brand-700",
	secondary: "bg-brand-50 text-brand-900 hover:bg-brand-100",
};

/**
 * Shared, Tailwind-styled button. Uses `motion` for a real (not decorative-only) hover/tap
 * micro-interaction, so this is the component in the library that demonstrates the `motion`
 * requirement concretely rather than by unused import.
 */
export function Button({
	variant = "primary",
	className = "",
	children,
	...rest
}: ButtonProps) {
	return (
		<motion.button
			whileHover={{ scale: 1.03 }}
			whileTap={{ scale: 0.97 }}
			transition={{ type: "spring", stiffness: 400, damping: 20 }}
			className={`inline-flex items-center justify-center rounded-card px-4 py-2 text-sm font-medium shadow-sm transition-colors ${variantClasses[variant]} ${className}`}
			{...rest}
		>
			{children}
		</motion.button>
	);
}

import type { HTMLAttributes, ReactNode } from "react";

export type BadgeTone = "neutral" | "success" | "warning" | "danger";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
	tone?: BadgeTone;
	children: ReactNode;
}

const toneClasses: Record<BadgeTone, string> = {
	neutral: "bg-surface-muted text-fg-muted",
	success: "bg-success-soft text-success",
	warning: "bg-warning-soft text-warning",
	danger: "bg-danger-soft text-danger",
};

/** Small status/role pill. */
export function Badge({
	tone = "neutral",
	className = "",
	children,
	...rest
}: BadgeProps) {
	return (
		<span
			className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${toneClasses[tone]} ${className}`}
			{...rest}
		>
			{children}
		</span>
	);
}

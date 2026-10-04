import type { HTMLAttributes, ReactNode } from "react";

export interface CardProps
	extends Omit<HTMLAttributes<HTMLDivElement>, "title"> {
	title?: ReactNode;
	children: ReactNode;
}

/**
 * Shared card shell. Composition-only (no animation of its own) — the library only needs
 * one component demonstrating `motion` (see `atoms/button`), not every component.
 */
export function Card({ title, className = "", children, ...rest }: CardProps) {
	return (
		<div
			className={`rounded-card border border-line bg-surface p-5 shadow-sm ${className}`}
			{...rest}
		>
			{title ? (
				<h2 className="mb-2 text-base font-semibold text-fg">{title}</h2>
			) : null}
			<div className="text-sm text-fg-muted">{children}</div>
		</div>
	);
}

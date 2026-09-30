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
			className={`rounded-card border border-brand-100 bg-white p-5 shadow-sm ${className}`}
			{...rest}
		>
			{title ? (
				<h3 className="mb-2 text-base font-semibold text-brand-900">{title}</h3>
			) : null}
			<div className="text-sm text-brand-700">{children}</div>
		</div>
	);
}

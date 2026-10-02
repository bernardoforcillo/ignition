import type { ReactNode } from "react";

export interface EmptyStateProps {
	title: string;
	description?: ReactNode;
	action?: ReactNode;
	className?: string;
}

/** Placeholder for a list or page with nothing to show yet, with an optional call to action. */
export function EmptyState({
	title,
	description,
	action,
	className = "",
}: EmptyStateProps) {
	return (
		<div
			className={`rounded-card border border-dashed border-line px-6 py-10 text-center ${className}`}
		>
			<p className="text-sm font-semibold text-fg">{title}</p>
			{description ? (
				<p className="mt-1 text-sm text-fg-muted">{description}</p>
			) : null}
			{action ? <div className="mt-4 flex justify-center">{action}</div> : null}
		</div>
	);
}

import type { HTMLAttributes } from "react";

export type SkeletonProps = HTMLAttributes<HTMLDivElement>;

/** Loading placeholder block; size it with utility classes (`h-4 w-32`). Hidden from assistive tech. */
export function Skeleton({ className = "", ...rest }: SkeletonProps) {
	return (
		<div
			aria-hidden="true"
			className={`animate-pulse rounded-card bg-surface-muted ${className}`}
			{...rest}
		/>
	);
}

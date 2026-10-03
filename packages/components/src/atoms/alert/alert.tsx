import type { HTMLAttributes, ReactNode } from "react";

export type AlertTone = "danger" | "success" | "info";

export interface AlertProps extends HTMLAttributes<HTMLDivElement> {
	tone?: AlertTone;
	children: ReactNode;
}

const toneClasses: Record<AlertTone, string> = {
	danger: "border-danger/40 bg-danger-soft text-danger",
	success: "border-success/40 bg-success-soft text-success",
	info: "border-line bg-surface-muted text-fg",
};

/** Inline message banner. Errors announce themselves (`role="alert"`), the rest are polite. */
export function Alert({
	tone = "info",
	className = "",
	children,
	...rest
}: AlertProps) {
	return (
		<div
			role={tone === "danger" ? "alert" : "status"}
			className={`rounded-card border px-3 py-2 text-sm ${toneClasses[tone]} ${className}`}
			{...rest}
		>
			{children}
		</div>
	);
}

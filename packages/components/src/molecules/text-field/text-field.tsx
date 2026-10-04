import { type ReactNode, useId, useState } from "react";

import { Input, type InputProps } from "../../atoms/input";

export interface TextFieldProps extends Omit<InputProps, "invalid"> {
	label: ReactNode;
	/** Error message; marks the field invalid and is announced via `aria-describedby`. */
	error?: string;
	hint?: ReactNode;
	/** Adds a "Show"/"Hide" toggle (use with `type="password"`). */
	revealable?: boolean;
}

/**
 * Label + input + hint + error wired together for accessibility: the label is bound with
 * `htmlFor`, and the hint/error ids feed `aria-describedby`.
 */
export function TextField({
	label,
	error,
	hint,
	revealable = false,
	id,
	type = "text",
	className = "",
	...rest
}: TextFieldProps) {
	const autoId = useId();
	const inputId = id ?? autoId;
	const hintId = `${inputId}-hint`;
	const errorId = `${inputId}-error`;
	const [revealed, setRevealed] = useState(false);
	const describedBy =
		[hint ? hintId : null, error ? errorId : null].filter(Boolean).join(" ") ||
		undefined;

	return (
		<div className={`space-y-1.5 ${className}`}>
			<label htmlFor={inputId} className="block text-sm font-medium text-fg">
				{label}
			</label>
			<div className="relative">
				<Input
					id={inputId}
					type={revealable && revealed ? "text" : type}
					invalid={Boolean(error)}
					aria-describedby={describedBy}
					className={revealable ? "pr-16" : ""}
					{...rest}
				/>
				{revealable ? (
					<button
						type="button"
						onClick={() => setRevealed((v) => !v)}
						aria-pressed={revealed}
						className="absolute inset-y-0 right-0 rounded-card px-3 text-xs font-medium text-fg-muted hover:text-fg focus-visible:outline-2 focus-visible:outline-brand-500"
					>
						{revealed ? "Hide" : "Show"}
						<span className="sr-only"> password</span>
					</button>
				) : null}
			</div>
			{hint ? (
				<p id={hintId} className="text-xs text-fg-muted">
					{hint}
				</p>
			) : null}
			{error ? (
				<p id={errorId} className="text-xs font-medium text-danger">
					{error}
				</p>
			) : null}
		</div>
	);
}

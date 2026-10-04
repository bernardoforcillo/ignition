import type { InputHTMLAttributes } from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
	/** Sets `aria-invalid` and the error border. */
	invalid?: boolean;
}

export const inputClasses =
	"w-full rounded-card border border-line bg-surface px-3 py-2 text-sm text-fg placeholder:text-fg-muted focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-500 aria-invalid:border-danger disabled:cursor-not-allowed disabled:opacity-60";

/** Styled text input. Always pair it with a visible label (see the `TextField` molecule). */
export function Input({ invalid, className = "", ...rest }: InputProps) {
	return (
		<input
			aria-invalid={invalid || undefined}
			className={`${inputClasses} ${className}`}
			{...rest}
		/>
	);
}

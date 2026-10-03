import { type ReactNode, type SelectHTMLAttributes, useId } from "react";

import { inputClasses } from "../../atoms/input";

export interface SelectOption {
	value: string;
	label: string;
}

export interface SelectFieldProps
	extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "children"> {
	label: ReactNode;
	options: readonly SelectOption[];
	error?: string;
}

/** Labelled native `<select>`: keyboard and screen-reader behavior come for free. */
export function SelectField({
	label,
	options,
	error,
	id,
	className = "",
	...rest
}: SelectFieldProps) {
	const autoId = useId();
	const selectId = id ?? autoId;
	const errorId = `${selectId}-error`;

	return (
		<div className={`space-y-1.5 ${className}`}>
			<label htmlFor={selectId} className="block text-sm font-medium text-fg">
				{label}
			</label>
			<select
				id={selectId}
				aria-invalid={error ? true : undefined}
				aria-describedby={error ? errorId : undefined}
				className={inputClasses}
				{...rest}
			>
				{options.map((option) => (
					<option key={option.value} value={option.value}>
						{option.label}
					</option>
				))}
			</select>
			{error ? (
				<p id={errorId} className="text-xs font-medium text-danger">
					{error}
				</p>
			) : null}
		</div>
	);
}

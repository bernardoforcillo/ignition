import { type ReactNode, useEffect, useId, useRef } from "react";

export interface DialogProps {
	open: boolean;
	onClose: () => void;
	title: ReactNode;
	description?: ReactNode;
	children: ReactNode;
}

/**
 * Modal built on the native `<dialog>` element, which supplies the focus trap, Escape to close
 * and the inert backdrop. Named by its title and described by `description`.
 */
export function Dialog({
	open,
	onClose,
	title,
	description,
	children,
}: DialogProps) {
	const ref = useRef<HTMLDialogElement>(null);
	const titleId = useId();
	const descriptionId = useId();

	useEffect(() => {
		const dialog = ref.current;
		if (!dialog) return;
		if (open && !dialog.open) dialog.showModal();
		if (!open && dialog.open) dialog.close();
	}, [open]);

	return (
		<dialog
			ref={ref}
			onClose={onClose}
			aria-labelledby={titleId}
			aria-describedby={description ? descriptionId : undefined}
			className="m-auto w-[calc(100%-2rem)] max-w-md rounded-card border border-line bg-surface p-5 text-fg shadow-xl backdrop:bg-fg/40"
		>
			<h2 id={titleId} className="text-lg font-semibold">
				{title}
			</h2>
			{description ? (
				<p id={descriptionId} className="mt-1 text-sm text-fg-muted">
					{description}
				</p>
			) : null}
			<div className="mt-4">{open ? children : null}</div>
		</dialog>
	);
}

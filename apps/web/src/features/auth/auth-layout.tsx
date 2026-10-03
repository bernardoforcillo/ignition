import type { ReactNode } from "react";

import { ThemeToggle } from "~/features/theme-toggle";

type Props = {
	title: string;
	description?: ReactNode;
	children: ReactNode;
	footer?: ReactNode;
};

/** Centered card shared by every signed-out screen. */
export function AuthLayout({ title, description, children, footer }: Props) {
	return (
		<div className="flex min-h-screen flex-col bg-surface-muted text-fg">
			<div className="flex justify-end px-4 py-3">
				<ThemeToggle />
			</div>
			<main className="flex flex-1 items-start justify-center px-4 pb-12 sm:items-center">
				<div className="w-full max-w-sm space-y-6 rounded-card border border-line bg-surface p-6 shadow-sm">
					<div className="space-y-1">
						<h1 className="text-xl font-semibold">{title}</h1>
						{description ? (
							<p className="text-sm text-fg-muted">{description}</p>
						) : null}
					</div>
					{children}
					{footer ? (
						<div className="text-center text-sm text-fg-muted">{footer}</div>
					) : null}
				</div>
			</main>
		</div>
	);
}

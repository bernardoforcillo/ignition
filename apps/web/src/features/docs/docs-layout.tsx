import { Outlet } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";

import {
	MAIN_CONTENT_ID,
	SiteFooter,
	SiteHeader,
} from "~/features/site-chrome";

import { DocsSidebar } from "./docs-sidebar";

const NAV_ID = "docs-nav";

/** Sidebar plus content. Below `lg` the sidebar is a drawer opened from the header's Menu button. */
export function DocsLayout() {
	const [open, setOpen] = useState(false);
	const menuButton = useRef<HTMLButtonElement>(null);
	const closeButton = useRef<HTMLButtonElement>(null);

	useEffect(() => {
		if (!open) return;
		closeButton.current?.focus();
		const onKey = (event: KeyboardEvent) => {
			if (event.key !== "Escape") return;
			setOpen(false);
			menuButton.current?.focus();
		};
		document.addEventListener("keydown", onKey);
		return () => document.removeEventListener("keydown", onKey);
	}, [open]);

	return (
		<>
			<SiteHeader
				leading={
					<button
						ref={menuButton}
						type="button"
						aria-expanded={open}
						aria-controls={NAV_ID}
						onClick={() => setOpen((value) => !value)}
						className="rounded-card border border-line px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500 lg:hidden"
					>
						Menu
					</button>
				}
			/>
			<div className="mx-auto flex max-w-6xl gap-10 px-4">
				{open ? (
					<button
						type="button"
						tabIndex={-1}
						aria-label="Close menu"
						onClick={() => setOpen(false)}
						className="fixed inset-0 z-40 bg-fg/40 lg:hidden"
					/>
				) : null}
				<aside
					id={NAV_ID}
					className={`${
						open
							? "fixed inset-y-0 left-0 z-50 block w-72 overflow-y-auto border-r border-line bg-surface p-4 shadow-xl"
							: "hidden"
					} lg:sticky lg:bottom-auto lg:top-14 lg:z-auto lg:block lg:h-[calc(100vh-3.5rem)] lg:w-60 lg:shrink-0 lg:overflow-y-auto lg:border-0 lg:bg-transparent lg:p-0 lg:py-8 lg:shadow-none`}
				>
					<div className="mb-3 flex items-center justify-between lg:hidden">
						<span className="text-sm font-semibold text-fg">Documentation</span>
						<button
							ref={closeButton}
							type="button"
							onClick={() => {
								setOpen(false);
								menuButton.current?.focus();
							}}
							className="rounded-card border border-line px-2 py-1 text-xs font-medium text-fg hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500"
						>
							Close
						</button>
					</div>
					<DocsSidebar onNavigate={() => setOpen(false)} />
				</aside>
				<main
					id={MAIN_CONTENT_ID}
					tabIndex={-1}
					className="min-w-0 flex-1 py-8 outline-none"
				>
					<Outlet />
				</main>
			</div>
			<SiteFooter />
		</>
	);
}

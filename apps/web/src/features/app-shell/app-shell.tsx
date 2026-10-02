import { Button } from "@ignition/components";
import { useQueryClient } from "@tanstack/react-query";
import {
	Link,
	Outlet,
	useRouter,
	useRouterState,
} from "@tanstack/react-router";
import { useEffect, useRef } from "react";

import { ThemeToggle } from "~/features/theme-toggle";
import { WorkspaceSwitcher } from "~/features/workspace";
import { signOut, useAuthStore } from "~/stores/auth";

const NAV = [
	{ to: "/app", label: "Overview", exact: true },
	{ to: "/app/members", label: "Members", exact: false },
	{ to: "/app/billing", label: "Billing", exact: false },
	{ to: "/app/settings", label: "Settings", exact: false },
] as const;

export function AppShell() {
	const router = useRouter();
	const queryClient = useQueryClient();
	const status = useAuthStore((state) => state.status);
	const email = useAuthStore((state) => state.user?.email);
	const href = useRouterState({ select: (s) => s.location.href });
	const signingOut = useRef(false);

	// The session can end under us (refresh token revoked): go to sign-in and come back here after.
	useEffect(() => {
		if (status === "anonymous" && !signingOut.current) {
			router.navigate({ to: "/login", search: { redirect: href } });
		}
	}, [status, href, router]);

	const onSignOut = async () => {
		signingOut.current = true;
		await signOut();
		queryClient.clear();
		router.navigate({ to: "/login", search: {} });
	};

	return (
		<div className="min-h-screen bg-surface-muted text-fg">
			<header className="border-b border-line bg-surface">
				<div className="mx-auto flex max-w-5xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
					<span className="text-base font-semibold">Ignition</span>
					<WorkspaceSwitcher />
					<div className="ml-auto flex items-center gap-2">
						<ThemeToggle />
						<Button variant="secondary" onClick={onSignOut} type="button">
							Sign out
						</Button>
					</div>
				</div>
				<nav
					aria-label="Main"
					className="mx-auto flex max-w-5xl gap-1 overflow-x-auto px-4"
				>
					{NAV.map((item) => (
						<Link
							key={item.to}
							to={item.to}
							activeOptions={{ exact: item.exact }}
							className="whitespace-nowrap border-b-2 border-transparent px-3 py-2 text-sm font-medium text-fg-muted hover:text-fg"
							activeProps={{
								className: "border-brand-600 text-fg",
								"aria-current": "page",
							}}
						>
							{item.label}
						</Link>
					))}
				</nav>
			</header>
			<main className="mx-auto max-w-5xl space-y-6 px-4 py-8">
				{email ? <p className="sr-only">Signed in as {email}</p> : null}
				<Outlet />
			</main>
		</div>
	);
}

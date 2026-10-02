import { redirect } from "@tanstack/react-router";

import { workspacesQuery } from "~/lib/api";
import { queryClient } from "~/lib/query";
import { safeRedirect } from "~/lib/redirect";
import { useAuthStore } from "~/stores/auth";

/** Signed-in users only. Anonymous visitors go to /login and come back to `href` afterwards. */
export async function requireAuth(href: string): Promise<void> {
	await useAuthStore.getState().restore();
	if (useAuthStore.getState().status !== "authenticated") {
		throw redirect({ to: "/login", search: { redirect: href } });
	}
}

/** Public-only pages (login, signup): signed-in users are sent on to where they were going. */
export async function redirectIfAuthenticated(target: unknown): Promise<void> {
	await useAuthStore.getState().restore();
	if (useAuthStore.getState().status === "authenticated") {
		// safeRedirect guarantees a same-origin relative path; split it back into router terms.
		const url = new URL(safeRedirect(target), "http://ignition.invalid");
		throw redirect({
			to: url.pathname as "/app",
			search: Object.fromEntries(url.searchParams) as never,
		});
	}
}

/** Resolves the caller's workspaces, sending users with none to onboarding. */
export async function requireWorkspace() {
	const workspaces = await queryClient.ensureQueryData(workspacesQuery());
	if (workspaces.length === 0) throw redirect({ to: "/onboarding" });
	return workspaces;
}

/** Onboarding is for users with no workspace yet. */
export async function redirectIfHasWorkspace(): Promise<void> {
	const workspaces = await queryClient.ensureQueryData(workspacesQuery());
	if (workspaces.length > 0) throw redirect({ to: "/app" });
}

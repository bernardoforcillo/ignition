import { getRouteApi } from "@tanstack/react-router";

import { LoginPanel } from "~/features/auth";

export function LoginPage() {
	const { redirect } = getRouteApi("/login").useSearch();
	return <LoginPanel redirectTo={redirect} />;
}

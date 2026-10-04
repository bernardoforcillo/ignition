import { getRouteApi } from "@tanstack/react-router";

import { VerifyEmailPanel } from "~/features/auth";

export function VerifyEmailPage() {
	const { token } = getRouteApi("/verify-email").useSearch();
	return <VerifyEmailPanel token={token} />;
}

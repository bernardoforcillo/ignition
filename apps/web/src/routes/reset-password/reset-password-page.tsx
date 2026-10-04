import { getRouteApi } from "@tanstack/react-router";

import { ResetPasswordPanel } from "~/features/auth";

export function ResetPasswordPage() {
	const { token } = getRouteApi("/reset-password").useSearch();
	return <ResetPasswordPanel token={token} />;
}

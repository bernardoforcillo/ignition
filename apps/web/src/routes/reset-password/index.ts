import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { rootRoute } from "~/routes/root";
import { stringParam } from "~/routes/root/search";

export const resetPasswordRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/reset-password",
	validateSearch: (search: Record<string, unknown>): { token?: string } => {
		const value = stringParam(search.token);
		return value ? { token: value } : {};
	},
	component: lazyRouteComponent(
		() => import("./reset-password-page"),
		"ResetPasswordPage",
	),
});

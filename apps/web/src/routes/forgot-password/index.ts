import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { rootRoute } from "~/routes/root";

export const forgotPasswordRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/forgot-password",
	component: lazyRouteComponent(
		() => import("~/features/auth"),
		"ForgotPasswordPanel",
	),
});

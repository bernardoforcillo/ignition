import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { redirectIfAuthenticated } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";
import { stringParam } from "~/routes/root/search";

export const loginRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/login",
	validateSearch: (search: Record<string, unknown>): { redirect?: string } => {
		const value = stringParam(search.redirect);
		return value ? { redirect: value } : {};
	},
	beforeLoad: ({ search }) => redirectIfAuthenticated(search.redirect),
	component: lazyRouteComponent(() => import("./login-page"), "LoginPage"),
});

import { createRoute } from "@tanstack/react-router";

import { redirectIfAuthenticated } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";
import { stringParam } from "~/routes/root/search";

import { LoginPage } from "./login-page";

export const loginRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/login",
	validateSearch: (search: Record<string, unknown>): { redirect?: string } => {
		const value = stringParam(search.redirect);
		return value ? { redirect: value } : {};
	},
	beforeLoad: ({ search }) => redirectIfAuthenticated(search.redirect),
	component: LoginPage,
});

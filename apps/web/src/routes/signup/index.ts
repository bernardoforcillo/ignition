import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { redirectIfAuthenticated } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const signupRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/signup",
	beforeLoad: () => redirectIfAuthenticated(undefined),
	component: lazyRouteComponent(() => import("~/features/auth"), "SignupPanel"),
});

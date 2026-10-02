import { createRoute } from "@tanstack/react-router";

import { SignupPanel } from "~/features/auth";
import { redirectIfAuthenticated } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const signupRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/signup",
	beforeLoad: () => redirectIfAuthenticated(undefined),
	component: SignupPanel,
});

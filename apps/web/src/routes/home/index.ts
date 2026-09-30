import { createRoute } from "@tanstack/react-router";

import { GreetingPanel } from "~/features/greeting";
import { rootRoute } from "~/routes/root";

export const homeRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/",
	component: GreetingPanel,
});

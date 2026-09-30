import { createRoute } from "@tanstack/react-router";

import { ShowcasePanel } from "~/features/showcase";
import { rootRoute } from "~/routes/root";

export const aboutRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/about",
	component: ShowcasePanel,
});

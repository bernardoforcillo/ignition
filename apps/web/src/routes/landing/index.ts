import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { restoreSession } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

/** The public landing page. The session is restored first so the CTA says "Open app" at once. */
export const landingRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/",
	beforeLoad: restoreSession,
	component: lazyRouteComponent(
		() => import("~/features/landing"),
		"LandingPage",
	),
});

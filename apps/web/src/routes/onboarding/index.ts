import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { redirectIfHasWorkspace, requireAuth } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const onboardingRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/onboarding",
	beforeLoad: async ({ location }) => {
		await requireAuth(location.href);
		await redirectIfHasWorkspace();
	},
	component: lazyRouteComponent(
		() => import("~/features/onboarding"),
		"OnboardingPanel",
	),
});

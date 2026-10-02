import { createRoute } from "@tanstack/react-router";

import { OnboardingPanel } from "~/features/onboarding";
import { redirectIfHasWorkspace, requireAuth } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const onboardingRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/onboarding",
	beforeLoad: async ({ location }) => {
		await requireAuth(location.href);
		await redirectIfHasWorkspace();
	},
	component: OnboardingPanel,
});

import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { requireAuth } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";
import { stringParam } from "~/routes/root/search";

/** Signed-out visitors sign in first, then return here (with the token) to accept. */
export const acceptInviteRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/invite/accept",
	validateSearch: (search: Record<string, unknown>): { token?: string } => {
		const value = stringParam(search.token);
		return value ? { token: value } : {};
	},
	beforeLoad: ({ location }) => requireAuth(location.href),
	component: lazyRouteComponent(
		() => import("./accept-invite-page"),
		"AcceptInvitePage",
	),
});

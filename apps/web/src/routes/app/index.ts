import { createRoute } from "@tanstack/react-router";

import { AppShell } from "~/features/app-shell";
import { BillingPanel } from "~/features/billing";
import { MembersPanel } from "~/features/members";
import { OverviewPanel } from "~/features/overview";
import { SettingsPanel } from "~/features/settings";
import { requireAuth, requireWorkspace } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const appRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/app",
	beforeLoad: async ({ location }) => {
		await requireAuth(location.href);
		await requireWorkspace();
	},
	component: AppShell,
});

const overviewRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/",
	component: OverviewPanel,
});

const membersRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/members",
	component: MembersPanel,
});

const billingRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/billing",
	component: BillingPanel,
});

const settingsRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/settings",
	component: SettingsPanel,
});

export const appRoutes = appRoute.addChildren([
	overviewRoute,
	membersRoute,
	billingRoute,
	settingsRoute,
]);

import { createRoute, lazyRouteComponent } from "@tanstack/react-router";

import { requireAuth, requireWorkspace } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const appRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/app",
	beforeLoad: async ({ location }) => {
		await requireAuth(location.href);
		await requireWorkspace();
	},
	component: lazyRouteComponent(
		() => import("~/features/app-shell"),
		"AppShell",
	),
});

const overviewRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/",
	component: lazyRouteComponent(
		() => import("~/features/overview"),
		"OverviewPanel",
	),
});

const membersRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/members",
	component: lazyRouteComponent(
		() => import("~/features/members"),
		"MembersPanel",
	),
});

const filesRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/files",
	component: lazyRouteComponent(() => import("~/features/files"), "FilesPanel"),
});

const billingRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/billing",
	component: lazyRouteComponent(
		() => import("~/features/billing"),
		"BillingPanel",
	),
});

const settingsRoute = createRoute({
	getParentRoute: () => appRoute,
	path: "/settings",
	component: lazyRouteComponent(
		() => import("~/features/settings"),
		"SettingsPanel",
	),
});

export const appRoutes = appRoute.addChildren([
	overviewRoute,
	membersRoute,
	filesRoute,
	billingRoute,
	settingsRoute,
]);

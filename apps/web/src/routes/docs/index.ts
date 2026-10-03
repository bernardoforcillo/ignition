import {
	createRoute,
	lazyRouteComponent,
	redirect,
} from "@tanstack/react-router";

import { restoreSession } from "~/lib/route-guards";
import { rootRoute } from "~/routes/root";

export const docsRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/docs",
	beforeLoad: restoreSession,
	component: lazyRouteComponent(() => import("~/features/docs"), "DocsLayout"),
});

const docsIndexRoute = createRoute({
	getParentRoute: () => docsRoute,
	path: "/",
	beforeLoad: () => {
		throw redirect({ to: "/docs/$slug", params: { slug: "overview" } });
	},
});

const docsPageRoute = createRoute({
	getParentRoute: () => docsRoute,
	path: "/$slug",
	component: lazyRouteComponent(() => import("~/features/docs"), "DocsPage"),
});

export const docsRoutes = docsRoute.addChildren([
	docsIndexRoute,
	docsPageRoute,
]);

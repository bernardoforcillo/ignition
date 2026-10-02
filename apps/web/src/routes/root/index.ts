import { createRootRoute, createRoute, redirect } from "@tanstack/react-router";

import { NotFoundPage, RootLayout } from "./root-layout";

export const rootRoute = createRootRoute({
	component: RootLayout,
	notFoundComponent: NotFoundPage,
});

/** `/` has no page of its own: the app guard decides between sign-in, onboarding and the app. */
export const indexRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/",
	beforeLoad: () => {
		throw redirect({ to: "/app" });
	},
});

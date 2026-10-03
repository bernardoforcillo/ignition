import { createRootRoute } from "@tanstack/react-router";

import { NotFoundPage, RootLayout } from "./root-layout";

export const rootRoute = createRootRoute({
	component: RootLayout,
	notFoundComponent: NotFoundPage,
});

import { createRootRoute } from "@tanstack/react-router";

import { RootLayout } from "./root-layout";

export const rootRoute = createRootRoute({
	component: RootLayout,
});

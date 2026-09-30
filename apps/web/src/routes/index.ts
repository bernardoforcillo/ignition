import { createRouter } from "@tanstack/react-router";

import { aboutRoute } from "~/routes/about";
import { homeRoute } from "~/routes/home";
import { rootRoute } from "~/routes/root";

const routeTree = rootRoute.addChildren([homeRoute, aboutRoute]);

export const router = createRouter({ routeTree });

// Register the router instance for full type-safety on <Link>, useNavigate, etc.
declare module "@tanstack/react-router" {
	interface Register {
		router: typeof router;
	}
}

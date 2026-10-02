import { createRoute } from "@tanstack/react-router";

import { ForgotPasswordPanel } from "~/features/auth";
import { rootRoute } from "~/routes/root";

export const forgotPasswordRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: "/forgot-password",
	component: ForgotPasswordPanel,
});

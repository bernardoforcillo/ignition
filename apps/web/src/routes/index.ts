import { createRouter } from "@tanstack/react-router";

import { appRoutes } from "~/routes/app";
import { forgotPasswordRoute } from "~/routes/forgot-password";
import { acceptInviteRoute } from "~/routes/invite";
import { loginRoute } from "~/routes/login";
import { onboardingRoute } from "~/routes/onboarding";
import { resetPasswordRoute } from "~/routes/reset-password";
import { indexRoute, rootRoute } from "~/routes/root";
import { signupRoute } from "~/routes/signup";
import { verifyEmailRoute } from "~/routes/verify-email";

const routeTree = rootRoute.addChildren([
	indexRoute,
	loginRoute,
	signupRoute,
	verifyEmailRoute,
	forgotPasswordRoute,
	resetPasswordRoute,
	acceptInviteRoute,
	onboardingRoute,
	appRoutes,
]);

export const router = createRouter({ routeTree });

// Register the router instance for full type-safety on <Link>, useNavigate, etc.
declare module "@tanstack/react-router" {
	interface Register {
		router: typeof router;
	}
}

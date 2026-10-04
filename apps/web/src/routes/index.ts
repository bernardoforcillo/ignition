import { createRouter } from "@tanstack/react-router";

import { RouteError } from "~/features/error-boundary";
import { appRoutes } from "~/routes/app";
import { docsRoutes } from "~/routes/docs";
import { forgotPasswordRoute } from "~/routes/forgot-password";
import { acceptInviteRoute } from "~/routes/invite";
import { landingRoute } from "~/routes/landing";
import { loginRoute } from "~/routes/login";
import { onboardingRoute } from "~/routes/onboarding";
import { resetPasswordRoute } from "~/routes/reset-password";
import { rootRoute } from "~/routes/root";
import { signupRoute } from "~/routes/signup";
import { verifyEmailRoute } from "~/routes/verify-email";

const routeTree = rootRoute.addChildren([
	landingRoute,
	docsRoutes,
	loginRoute,
	signupRoute,
	verifyEmailRoute,
	forgotPasswordRoute,
	resetPasswordRoute,
	acceptInviteRoute,
	onboardingRoute,
	appRoutes,
]);

export const router = createRouter({
	routeTree,
	defaultPreload: "intent",
	defaultErrorComponent: ({ error }) => RouteError({ error }),
});

// Register the router instance for full type-safety on <Link>, useNavigate, etc.
declare module "@tanstack/react-router" {
	interface Register {
		router: typeof router;
	}
}

import { useEffect } from "react";

import { analytics, identityAction } from "~/lib/analytics";
import { router } from "~/routes";
import { useAuthStore } from "~/stores/auth";
import { useConsentStore } from "~/stores/consent";

/**
 * Connects analytics to the three things that drive it: the visitor's consent, route changes
 * (single-page apps never reload, so pageviews are sent from the router) and the signed-in
 * account (identified by its id only, never an email; reset on sign-out so the next visitor on
 * this browser is a new person).
 */
export function useAnalyticsBridge(): void {
	const decision = useConsentStore((state) => state.decision);

	useEffect(() => {
		analytics.applyConsent(decision);
	}, [decision]);

	useEffect(
		() =>
			router.subscribe("onResolved", ({ toLocation }) => {
				analytics.pageview(toLocation.pathname);
			}),
		[],
	);

	useEffect(() => {
		const current = useAuthStore.getState().user?.id ?? "";
		if (current) analytics.identify(current);
		return useAuthStore.subscribe((state, previous) => {
			const next = state.user?.id ?? "";
			const action = identityAction(previous.user?.id ?? "", next);
			if (action === "identify") analytics.identify(next);
			else if (action === "reset") analytics.reset();
		});
	}, []);
}

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
		// The SDK starts only now, so a sign-in that happened before consent was not identified.
		const current = useAuthStore.getState().user?.id ?? "";
		if (decision === "granted" && current) analytics.identify(current);
	}, [decision]);

	useEffect(() => {
		// Route components are lazy, so the first navigation can resolve before this effect
		// subscribes (more often on a fast machine). Report the page already shown, otherwise a
		// full page load would never produce its pageview. While the router is still resolving,
		// the subscription below reports it.
		if (router.state.status === "idle" && router.state.resolvedLocation) {
			analytics.pageview(router.state.resolvedLocation.pathname);
		}
		return router.subscribe("onResolved", ({ toLocation }) => {
			analytics.pageview(toLocation.pathname);
		});
	}, []);

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

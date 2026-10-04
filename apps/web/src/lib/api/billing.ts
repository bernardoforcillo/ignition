import { queryOptions } from "@tanstack/react-query";

import { billingClient } from "~/lib/rpc";

export const subscriptionQuery = (workspaceId: string) =>
	queryOptions({
		queryKey: ["billing", workspaceId, "subscription"],
		queryFn: () => billingClient.getSubscription({ workspaceId }),
	});

export const pricesQuery = () =>
	queryOptions({
		queryKey: ["billing", "prices"],
		queryFn: async () => (await billingClient.listPrices({})).prices,
	});

export const startCheckout = async (workspaceId: string, priceId: string) =>
	(await billingClient.startCheckout({ workspaceId, priceId })).url;

export const openPortal = async (workspaceId: string) =>
	(await billingClient.openPortal({ workspaceId })).url;

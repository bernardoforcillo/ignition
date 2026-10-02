import { createHmac, randomUUID } from "node:crypto";

import { env, PLAN_PRICE_ID, WEBHOOK_SECRET } from "../harness/env";
import type { StripeCall } from "../harness/fake-stripe";

export type { StripeCall };

/** The requests the gateway has made to the fake Stripe API so far. */
export async function stripeCalls(): Promise<StripeCall[]> {
	return (await (
		await fetch(`${env.stripeUrl}/__calls`)
	).json()) as StripeCall[];
}

export interface SubscriptionEvent {
	/** `customer.subscription.created` | `.updated` | `.deleted`. */
	type: string;
	workspaceId: string;
	/** Stripe's status; `canceled` for a deleted subscription. */
	status?: string;
	eventId?: string;
	customerId?: string;
	priceIds?: string[];
}

/** The JSON Stripe would POST for a subscription event (only the fields the adapter parses). */
export function subscriptionPayload(e: SubscriptionEvent): string {
	const periodEnd = Math.floor(Date.now() / 1000) + 30 * 24 * 3600;
	return JSON.stringify({
		id: e.eventId ?? `evt_${randomUUID()}`,
		type: e.type,
		created: Math.floor(Date.now() / 1000),
		data: {
			object: {
				id: `sub_${e.workspaceId}`,
				customer: e.customerId ?? `cus_${e.workspaceId}`,
				status: e.status ?? "active",
				metadata: { workspace_id: e.workspaceId },
				current_period_start: Math.floor(Date.now() / 1000),
				current_period_end: periodEnd,
				items: {
					data: (e.priceIds ?? [PLAN_PRICE_ID]).map((id) => ({
						price: { id },
					})),
				},
			},
		},
	});
}

/** `Stripe-Signature: t=<unix>,v1=<hex HMAC-SHA256 of "t.payload">`. */
export function stripeSignature(
	payload: string,
	secret = WEBHOOK_SECRET,
	timestamp = Math.floor(Date.now() / 1000),
): string {
	const v1 = createHmac("sha256", secret)
		.update(`${timestamp}.${payload}`)
		.digest("hex");
	return `t=${timestamp},v1=${v1}`;
}

/** POSTs a webhook to the gateway; the caller decides how it is signed. */
export function postWebhook(
	payload: string,
	signature: string,
): Promise<Response> {
	return fetch(`${env.gatewayUrl}/webhooks/stripe`, {
		method: "POST",
		headers: {
			"content-type": "application/json",
			"stripe-signature": signature,
		},
		body: payload,
	});
}

/** Signs `event` with the gateway's secret and delivers it. */
export function sendSubscriptionEvent(e: SubscriptionEvent): Promise<Response> {
	const payload = subscriptionPayload(e);
	return postWebhook(payload, stripeSignature(payload));
}

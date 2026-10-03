import { ADDON_PRICE_ID, env, PLAN_PRICE_ID } from "../src/harness/env";
import {
	postWebhook,
	sendSubscriptionEvent,
	stripeCalls,
	stripeSignature,
	subscriptionPayload,
} from "../src/support/stripe";
import { expect, test } from "../src/support/test";
import { currentWorkspaceId } from "../src/support/users";

test("billing: checkout, signed webhooks, portal, cancelation", async ({
	workspaceUser,
}) => {
	const { page } = await workspaceUser("Billing Co");
	const workspaceId = await currentWorkspaceId(page);
	await page.getByRole("link", { name: "Billing" }).click();
	await expect(
		page.getByRole("heading", { name: "Billing", level: 1 }),
	).toBeVisible();

	// Free plan, with the plans and add-ons the gateway lists; nothing to manage yet.
	await expect(page.getByText("Free", { exact: true })).toBeVisible();
	await expect(page.getByRole("button", { name: "Choose Pro" })).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Choose Extra api calls" }),
	).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Manage billing" }),
	).toHaveCount(0);

	// Checkout: the browser leaves for the (fake) hosted page.
	await page.getByRole("button", { name: "Choose Pro" }).click();
	await page.waitForURL(`${env.stripeUrl}/pay/**`);
	await expect(
		page.getByRole("heading", { name: "Fake Stripe Checkout" }),
	).toBeVisible();

	const checkout = (await stripeCalls()).find(
		(call) => call.form.client_reference_id === workspaceId,
	);
	expect(checkout?.path).toBe("/v1/checkout/sessions");
	expect(checkout?.authorization).toBe("Bearer sk_test_x");
	expect(checkout?.form).toMatchObject({
		mode: "subscription",
		"line_items[0][price]": PLAN_PRICE_ID,
		"line_items[0][quantity]": "1",
		client_reference_id: workspaceId,
		"subscription_data[metadata][workspace_id]": workspaceId,
	});
	// Return URLs are built by the gateway from APP_URL, never taken from the client.
	expect(checkout?.form.success_url).toMatch(new RegExp(`^${env.webUrl}/`));
	expect(checkout?.form.cancel_url).toMatch(new RegExp(`^${env.webUrl}/`));

	// The payment "happens": Stripe tells the gateway with a signed webhook.
	const eventId = `evt_${workspaceId}_created`;
	const created = subscriptionPayload({
		type: "customer.subscription.created",
		workspaceId,
		eventId,
	});
	const delivered = await postWebhook(created, stripeSignature(created));
	expect(delivered.status).toBe(200);

	await page.goto("/app/billing");
	await expect(page.getByText("Pro", { exact: true }).first()).toBeVisible();
	await expect(page.getByText("active", { exact: true })).toBeVisible();
	await expect(page.getByText("Current", { exact: true })).toBeVisible();
	await expect(page.getByRole("button", { name: "Choose Pro" })).toHaveCount(0);
	await expect(
		page.getByRole("button", { name: "Manage billing" }),
	).toBeVisible();

	// A forged signature is refused and changes nothing, even for a plan-removing event.
	const cancelation = subscriptionPayload({
		type: "customer.subscription.deleted",
		workspaceId,
		status: "canceled",
		eventId: `evt_${workspaceId}_forged`,
	});
	expect(
		(
			await postWebhook(
				cancelation,
				stripeSignature(cancelation, "whsec_wrong"),
			)
		).status,
	).toBe(400);
	expect(
		(await postWebhook(cancelation, stripeSignature(created))).status,
	).toBe(400); // signature of another body
	expect((await postWebhook(cancelation, "")).status).toBe(400);
	expect(
		(
			await postWebhook(
				cancelation,
				stripeSignature(cancelation, undefined, 1_000_000),
			)
		).status,
	).toBe(400); // correct secret, stale timestamp
	await page.reload();
	await expect(
		page.getByRole("button", { name: "Manage billing" }),
	).toBeVisible();
	await expect(page.getByText("Current", { exact: true })).toBeVisible();

	// The portal.
	await page.getByRole("button", { name: "Manage billing" }).click();
	await page.waitForURL(`${env.stripeUrl}/portal/**`);
	const portal = (await stripeCalls()).find(
		(call) =>
			call.path === "/v1/billing_portal/sessions" &&
			call.form.customer === `cus_${workspaceId}`,
	);
	expect(portal?.form.return_url).toMatch(new RegExp(`^${env.webUrl}/`));

	// Cancelation: back to Free, and the portal stays reachable (the customer still exists).
	expect(
		(
			await sendSubscriptionEvent({
				type: "customer.subscription.deleted",
				workspaceId,
				status: "canceled",
			})
		).status,
	).toBe(200);
	await page.goto("/app/billing");
	await expect(page.getByRole("button", { name: "Choose Pro" })).toBeVisible();
	await expect(page.getByText("Free", { exact: true })).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Manage billing" }),
	).toBeVisible();

	// Replaying the very first event (same id) is acknowledged but not applied again.
	expect((await postWebhook(created, stripeSignature(created))).status).toBe(
		200,
	);
	await page.reload();
	await expect(page.getByRole("button", { name: "Choose Pro" })).toBeVisible();
	await expect(page.getByText("Free", { exact: true })).toBeVisible();
});

test("billing: an add-on shows once its price is subscribed", async ({
	workspaceUser,
}) => {
	const { page } = await workspaceUser("Addon Co");
	const workspaceId = await currentWorkspaceId(page);
	expect(
		(
			await sendSubscriptionEvent({
				type: "customer.subscription.created",
				workspaceId,
				priceIds: [PLAN_PRICE_ID, ADDON_PRICE_ID],
			})
		).status,
	).toBe(200);
	await page.getByRole("link", { name: "Billing" }).click();
	await expect(
		page.getByText("Extra api calls", { exact: true }).first(),
	).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Choose Extra api calls" }),
	).toHaveCount(0);
});

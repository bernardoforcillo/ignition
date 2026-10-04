import { randomUUID } from "node:crypto";

import { type Fake, html, json, startServer } from "./http.js";

export interface StripeCall {
	path: string;
	authorization: string;
	form: Record<string, string>;
}

/**
 * Stands in for the two Stripe REST calls the Go adapter makes (form-encoded POSTs with a bearer
 * key): Checkout Sessions and Billing Portal sessions. Each answers with a hosted-page URL on this
 * server. `GET /__calls` returns what was received, so a test can assert what the gateway sent.
 */
export function startFakeStripe(apiKey: string): Promise<Fake> {
	const calls: StripeCall[] = [];
	let base = "";
	const server = startServer((req, res, body) => {
		const url = new URL(req.url ?? "/", "http://fake");
		if (req.method === "POST") {
			const kind =
				url.pathname === "/v1/checkout/sessions"
					? "pay"
					: url.pathname === "/v1/billing_portal/sessions"
						? "portal"
						: null;
			if (!kind) return json(res, 404, { error: { message: "unknown path" } });
			const authorization = req.headers.authorization ?? "";
			if (authorization !== `Bearer ${apiKey}`) {
				return json(res, 401, { error: { message: "Invalid API Key" } });
			}
			calls.push({
				path: url.pathname,
				authorization,
				form: Object.fromEntries(new URLSearchParams(body.toString("utf8"))),
			});
			const id = `${kind === "pay" ? "cs_test_" : "bps_"}${randomUUID().slice(0, 12)}`;
			return json(res, 200, { id, url: `${base}/${kind}/${id}` });
		}
		if (req.method === "GET" && url.pathname === "/__calls") {
			return json(res, 200, calls);
		}
		if (req.method === "GET" && url.pathname.startsWith("/pay/")) {
			return html(res, "Fake Stripe Checkout");
		}
		if (req.method === "GET" && url.pathname.startsWith("/portal/")) {
			return html(res, "Fake Stripe Portal");
		}
		json(res, 404, { error: { message: "not found" } });
	});
	return server.then((fake) => {
		base = fake.url;
		return fake;
	});
}

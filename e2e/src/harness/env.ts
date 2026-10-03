/**
 * What the harness publishes to the test workers. Playwright forks workers after global setup, so
 * variables set there are inherited; the config only pre-allocates the two web ports it needs for
 * `baseURL`.
 */
export const PLAN_PRICE_ID = "price_pro";
export const ADDON_PRICE_ID = "price_extra";
export const WEBHOOK_SECRET = "whsec_test";
export const RESEND_KEY = "re_test";
export const STRIPE_KEY = "sk_test_x";
export const POSTHOG_KEY = "phc_e2e_fake_key";
export const STORAGE_BUCKET = "e2e-files";
export const STORAGE_REGION = "e2e-region";
export const STORAGE_ACCESS_KEY_ID = "AKIAE2EFAKEKEY";
export const STORAGE_SECRET_ACCESS_KEY =
	"e2e-fake-secret-access-key-0123456789";

const required = (name: string): string => {
	const value = process.env[name];
	if (!value) {
		throw new Error(
			`${name} is not set: run the suite through Playwright (it starts the stack)`,
		);
	}
	return value;
};

export const env = {
	get gatewayUrl() {
		return required("E2E_GATEWAY_URL");
	},
	get resendUrl() {
		return required("E2E_RESEND_URL");
	},
	get stripeUrl() {
		return required("E2E_STRIPE_URL");
	},
	get storageUrl() {
		return required("E2E_STORAGE_URL");
	},
	get posthogUrl() {
		return required("E2E_POSTHOG_URL");
	},
	get webUrl() {
		return required("E2E_WEB_URL");
	},
	get webAnalyticsUrl() {
		return required("E2E_WEB_ANALYTICS_URL");
	},
};

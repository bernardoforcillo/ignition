import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { GatewayService } from "@ignition/proto/gateway/v1/gateway_pb";
import { AccountService } from "@ignition/proto/saas/v1/account_pb";
import { AuthService } from "@ignition/proto/saas/v1/auth_pb";
import { BillingService } from "@ignition/proto/saas/v1/billing_pb";
import { FeatureService } from "@ignition/proto/saas/v1/feature_pb";
import { FileService } from "@ignition/proto/saas/v1/file_pb";
import { WorkspaceService } from "@ignition/proto/saas/v1/workspace_pb";

import { createAuthInterceptor, type Session } from "./auth-interceptor";

export type { Session } from "./auth-interceptor";

/**
 * The auth store depends on these clients, so the interceptor cannot import the store back.
 * The store registers itself here once (see `stores/auth`); until then calls go out anonymous.
 */
let session: Session | null = null;
export const bindSession = (next: Session): void => {
	session = next;
};

/**
 * Typed Connect clients for the gateway, built from the shared `@ignition/proto` package
 * (generated from /proto, the same contract the Go services implement). `VITE_API_URL` is the
 * gateway origin; unset, requests go to the page's own origin (the Vite dev/preview proxy or the
 * Ingress), so the browser needs no CORS.
 */
const transport = createConnectTransport({
	baseUrl: import.meta.env.VITE_API_URL ?? window.location.origin,
	interceptors: [
		createAuthInterceptor({
			getAccessToken: () => session?.getAccessToken() ?? null,
			refresh: async () => (await session?.refresh()) ?? false,
		}),
	],
});

export const gatewayClient = createClient(GatewayService, transport);
export const authClient = createClient(AuthService, transport);
export const accountClient = createClient(AccountService, transport);
export const workspaceClient = createClient(WorkspaceService, transport);
export const featureClient = createClient(FeatureService, transport);
export const billingClient = createClient(BillingService, transport);
export const fileClient = createClient(FileService, transport);

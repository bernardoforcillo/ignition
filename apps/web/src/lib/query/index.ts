import { QueryClient } from "@tanstack/react-query";

/**
 * One shared client so non-React code (route guards, the auth store) can read and clear the same
 * cache the components use. Retrying a failed RPC is the interceptor's job, not the cache's.
 */
export const queryClient = new QueryClient({
	defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
});

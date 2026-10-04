import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

import { queryClient } from "~/lib/query";
import { router } from "~/routes";

import { useAnalyticsBridge } from "./analytics-bridge";

/**
 * App-root provider composition: TanStack Query's client above TanStack Router's provider.
 * Kept as its own module (rather than inlined in main.tsx) so main.tsx stays a pure bootstrap.
 */
export function App() {
	useAnalyticsBridge();
	return (
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>
	);
}

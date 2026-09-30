import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

import { router } from "~/routes";

const queryClient = new QueryClient();

/**
 * App-root provider composition: TanStack Query's client above TanStack Router's provider.
 * Kept as its own module (rather than inlined in main.tsx) so main.tsx stays a pure bootstrap.
 */
export function App() {
	return (
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>
	);
}

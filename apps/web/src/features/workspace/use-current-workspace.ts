import { useQuery } from "@tanstack/react-query";

import { workspacesQuery } from "~/lib/api";
import { useAuthStore } from "~/stores/auth";

/**
 * The workspace the app is showing: the persisted selection if the user still belongs to it,
 * otherwise their first one. The route guard guarantees at least one exists inside /app.
 */
export function useCurrentWorkspace() {
	const query = useQuery(workspacesQuery());
	const selectedId = useAuthStore((state) => state.workspaceId);
	const select = useAuthStore((state) => state.setWorkspaceId);

	const memberships = query.data ?? [];
	const current =
		memberships.find((m) => m.workspace?.id === selectedId) ?? memberships[0];

	return {
		isLoading: query.isLoading,
		memberships,
		workspace: current?.workspace,
		roleKey: current?.roleKey,
		select,
	};
}

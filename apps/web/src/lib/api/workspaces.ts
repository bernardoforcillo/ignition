import { queryOptions } from "@tanstack/react-query";

import { queryClient } from "~/lib/query";
import { workspaceClient } from "~/lib/rpc";

export const workspacesQuery = () =>
	queryOptions({
		queryKey: ["workspaces"],
		queryFn: async () => (await workspaceClient.listWorkspaces({})).workspaces,
	});

/** Re-reads the caller's workspaces from the server, bypassing the cache. */
export const refreshWorkspaces = () =>
	queryClient.fetchQuery({ ...workspacesQuery(), staleTime: 0 });

export const membersQuery = (workspaceId: string) =>
	queryOptions({
		queryKey: ["workspaces", workspaceId, "members"],
		queryFn: async () =>
			(await workspaceClient.listMembers({ workspaceId })).members,
	});

export const createWorkspace = async (name: string, slug: string) => {
	const res = await workspaceClient.createWorkspace({ name, slug });
	if (!res.workspace) throw new Error("CreateWorkspace returned no workspace");
	return res.workspace;
};

export const inviteMember = (
	workspaceId: string,
	email: string,
	roleKey: string,
) => workspaceClient.inviteMember({ workspaceId, email, roleKey });

export const acceptInvite = async (token: string) =>
	(await workspaceClient.acceptInvite({ token })).workspace;

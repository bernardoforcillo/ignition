import { Alert, Skeleton } from "@ignition/components";
import { useQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { useEffect } from "react";

import { acceptInvite, refreshWorkspaces } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { useAuthStore } from "~/stores/auth";

import { AuthLayout } from "../auth";

type Props = { token?: string };

/**
 * Joins the signed-in user to the workspace behind an invitation token, then opens the app.
 * Signed-out visitors never get here: the route guard sends them to /login and back.
 * A query (not an effect) so StrictMode's double mount cannot spend the single-use token twice.
 */
export function AcceptInvitePanel({ token }: Props) {
	const router = useRouter();
	const setWorkspaceId = useAuthStore((state) => state.setWorkspaceId);

	const query = useQuery({
		queryKey: ["accept-invite", token],
		queryFn: async () => {
			const workspace = await acceptInvite(token ?? "");
			await refreshWorkspaces();
			return workspace;
		},
		enabled: Boolean(token),
		retry: false,
		staleTime: Number.POSITIVE_INFINITY,
	});

	const joined = query.data;
	useEffect(() => {
		if (!query.isSuccess) return;
		if (joined?.id) setWorkspaceId(joined.id);
		router.history.push("/app");
	}, [query.isSuccess, joined, setWorkspaceId, router]);

	if (!token || query.isError) {
		return (
			<AuthLayout
				title="Invitation"
				footer={
					<Link to="/app" className="font-medium text-brand-600 underline">
						Go to the app
					</Link>
				}
			>
				<Alert tone="danger">
					{token
						? errorMessage(query.error, "token")
						: "This link is missing its token. Open the link from your email again."}
				</Alert>
			</AuthLayout>
		);
	}

	return (
		<AuthLayout title="Joining workspace">
			<div role="status" className="space-y-2">
				<span className="sr-only">Accepting your invitation…</span>
				<Skeleton className="h-4 w-full" />
				<Skeleton className="h-4 w-2/3" />
			</div>
		</AuthLayout>
	);
}

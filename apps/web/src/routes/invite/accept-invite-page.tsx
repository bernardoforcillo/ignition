import { getRouteApi } from "@tanstack/react-router";

import { AcceptInvitePanel } from "~/features/invite";

export function AcceptInvitePage() {
	const { token } = getRouteApi("/invite/accept").useSearch();
	return <AcceptInvitePanel token={token} />;
}

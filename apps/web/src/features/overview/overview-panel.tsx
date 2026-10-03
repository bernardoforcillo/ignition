import { Badge, Card, Skeleton } from "@ignition/components";
import { Link } from "@tanstack/react-router";

import { useCurrentWorkspace } from "~/features/workspace";

export function OverviewPanel() {
	const { workspace, roleKey, isLoading } = useCurrentWorkspace();

	if (isLoading || !workspace) {
		return (
			<div className="space-y-4">
				<Skeleton className="h-8 w-64" />
				<Skeleton className="h-32 w-full" />
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<h1 className="text-2xl font-semibold">{workspace.name}</h1>
			<div className="grid gap-4 sm:grid-cols-2">
				<Card title="Workspace">
					<dl className="space-y-2">
						<div className="flex justify-between gap-4">
							<dt>URL</dt>
							<dd className="font-mono text-fg">{workspace.slug}</dd>
						</div>
						<div className="flex justify-between gap-4">
							<dt>Your role</dt>
							<dd>
								<Badge>{roleKey}</Badge>
							</dd>
						</div>
					</dl>
				</Card>
				<Card title="Get started">
					<ul className="list-inside list-disc space-y-1">
						<li>
							<Link
								to="/app/members"
								className="font-medium text-brand-600 underline"
							>
								Invite your team
							</Link>
						</li>
						<li>
							<Link
								to="/app/billing"
								className="font-medium text-brand-600 underline"
							>
								Choose a plan
							</Link>
						</li>
					</ul>
				</Card>
			</div>
		</div>
	);
}

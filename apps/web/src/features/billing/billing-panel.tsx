import {
	Alert,
	Badge,
	Button,
	Card,
	EmptyState,
	Skeleton,
} from "@ignition/components";
import { useMutation, useQuery } from "@tanstack/react-query";

import { useCurrentWorkspace } from "~/features/workspace";
import { analytics } from "~/lib/analytics";
import {
	openPortal,
	pricesQuery,
	startCheckout,
	subscriptionQuery,
} from "~/lib/api";
import { errorMessage, isBillingDisabled } from "~/lib/errors";
import { isHttpUrl } from "~/lib/redirect";

import { displayName, formatDate } from "./format";

const goTo = (url: string) => {
	if (!isHttpUrl(url)) throw new Error("Unexpected redirect URL");
	window.location.assign(url);
};

export function BillingPanel() {
	const { workspace } = useCurrentWorkspace();
	const workspaceId = workspace?.id ?? "";

	const subscription = useQuery({
		...subscriptionQuery(workspaceId),
		enabled: Boolean(workspaceId),
	});
	const prices = useQuery({ ...pricesQuery(), enabled: Boolean(workspaceId) });

	const checkout = useMutation({
		mutationFn: async (priceId: string) =>
			goTo(await startCheckout(workspaceId, priceId)),
	});
	const portal = useMutation({
		mutationFn: async () => {
			analytics.track("billing_portal_opened", { location: "billing" });
			goTo(await openPortal(workspaceId));
		},
	});

	const choose = (priceId: string) => {
		const price = prices.data?.find((p) => p.priceId === priceId);
		analytics.track("checkout_started", {
			location: "billing",
			kind: price?.kind ?? "unknown",
			target_id: price?.id ?? "unknown",
		});
		checkout.mutate(priceId);
	};

	const loadError = subscription.error ?? prices.error;

	let body: React.ReactNode;
	if (loadError && isBillingDisabled(loadError)) {
		body = (
			<EmptyState
				title="Billing is not enabled"
				description="This deployment isn't set up to take payments."
			/>
		);
	} else if (loadError) {
		body = (
			<Alert tone="danger">
				{errorMessage(loadError)}{" "}
				<button
					type="button"
					className="font-medium underline"
					onClick={() => {
						subscription.refetch();
						prices.refetch();
					}}
				>
					Try again
				</button>
			</Alert>
		);
	} else if (subscription.isLoading || prices.isLoading || !subscription.data) {
		body = (
			<div role="status" className="space-y-4">
				<span className="sr-only">Loading billing…</span>
				<Skeleton className="h-32 w-full" />
				<Skeleton className="h-32 w-full" />
			</div>
		);
	} else {
		const sub = subscription.data;
		const renews = formatDate(sub.currentPeriodEnd);
		const plans = (prices.data ?? []).filter((p) => p.kind === "plan");
		const addOns = (prices.data ?? []).filter((p) => p.kind === "addon");
		const actionError = checkout.error ?? portal.error;

		body = (
			<>
				{actionError ? (
					<Alert tone="danger">{errorMessage(actionError)}</Alert>
				) : null}

				<Card title="Current plan">
					<div className="flex flex-wrap items-center justify-between gap-4">
						<div className="space-y-2">
							<p className="text-lg font-semibold text-fg">
								{displayName(sub.planId || "free")}
							</p>
							<div className="flex flex-wrap items-center gap-2">
								{sub.status ? (
									<Badge
										tone={
											sub.status === "active" || sub.status === "trialing"
												? "success"
												: "warning"
										}
									>
										{sub.status.replace("_", " ")}
									</Badge>
								) : null}
								{sub.addOnIds.map((id) => (
									<Badge key={id}>{displayName(id)}</Badge>
								))}
							</div>
							{renews ? <p>Current period ends {renews}.</p> : null}
						</div>
						{sub.canManage ? (
							<Button
								variant="secondary"
								loading={portal.isPending}
								onClick={() => portal.mutate()}
							>
								Manage billing
							</Button>
						) : null}
					</div>
				</Card>

				<PriceSection
					heading="Plans"
					items={plans}
					ownedIds={[sub.planId]}
					pendingId={checkout.isPending ? checkout.variables : undefined}
					onChoose={choose}
				/>
				{addOns.length > 0 ? (
					<PriceSection
						heading="Add-ons"
						items={addOns}
						ownedIds={sub.addOnIds}
						pendingId={checkout.isPending ? checkout.variables : undefined}
						onChoose={choose}
					/>
				) : null}
			</>
		);
	}

	return (
		<div className="space-y-6">
			<h1 className="text-2xl font-semibold">Billing</h1>
			{body}
		</div>
	);
}

type PriceItem = { priceId: string; id: string };

function PriceSection({
	heading,
	items,
	ownedIds,
	pendingId,
	onChoose,
}: {
	heading: string;
	items: PriceItem[];
	ownedIds: string[];
	pendingId: string | undefined;
	onChoose: (priceId: string) => void;
}) {
	return (
		<section aria-labelledby={`${heading}-heading`} className="space-y-3">
			<h2 id={`${heading}-heading`} className="text-lg font-semibold">
				{heading}
			</h2>
			{items.length === 0 ? (
				<EmptyState title={`No ${heading.toLowerCase()} available`} />
			) : (
				<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
					{items.map((item) => {
						const owned = ownedIds.includes(item.id);
						const name = displayName(item.id);
						return (
							<Card key={item.priceId} title={name}>
								{owned ? (
									<Badge tone="success">Current</Badge>
								) : (
									<Button
										loading={pendingId === item.priceId}
										onClick={() => onChoose(item.priceId)}
									>
										Choose {name}
									</Button>
								)}
							</Card>
						);
					})}
				</div>
			)}
		</section>
	);
}

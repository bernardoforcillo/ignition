import { useQuery } from "@tanstack/react-query";
import moment from "moment";
import { useEffect, useState } from "react";

import { fetchGreeting } from "~/lib/api";

/**
 * Exercises the `src/lib/api` transport layer through TanStack Query (loading/error/success +
 * refetch), and demonstrates `moment` doing something real: a relative timestamp ("a few seconds
 * ago") that keeps advancing on its own via a local tick, not a static, one-time format call.
 */
export function GreetingPanel() {
	const { data, isLoading, isError, refetch, isFetching } = useQuery({
		queryKey: ["greeting"],
		queryFn: fetchGreeting,
	});

	// Local re-render tick so the moment().fromNow() string below keeps advancing live, instead of
	// being computed once at fetch time and going stale on screen.
	const [, setTick] = useState(0);
	useEffect(() => {
		const id = window.setInterval(() => setTick((n) => n + 1), 1000);
		return () => window.clearInterval(id);
	}, []);

	return (
		<section className="space-y-3 rounded-card border border-brand-100 bg-brand-50 p-5">
			<h2 className="text-lg font-semibold text-brand-900">Server greeting</h2>

			{isLoading ? <p className="text-sm text-brand-700">Loading…</p> : null}
			{isError ? (
				<p className="text-sm text-red-600">Could not load a greeting.</p>
			) : null}

			{data ? (
				<div className="space-y-1">
					<p className="text-sm text-brand-900">{data.message}</p>
					<p className="text-xs text-brand-700">
						fetched {moment(data.fetchedAt).fromNow()} (
						{moment(data.fetchedAt).format("HH:mm:ss")})
					</p>
				</div>
			) : null}

			<button
				type="button"
				onClick={() => refetch()}
				disabled={isFetching}
				className="rounded-card bg-brand-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-brand-700 disabled:opacity-50"
			>
				{isFetching ? "Refreshing…" : "Refresh"}
			</button>
		</section>
	);
}

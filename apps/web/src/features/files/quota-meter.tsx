import { formatBytes, usagePercent } from "./format";

type Props = {
	usedBytes: number;
	/** Undefined means the plan has no storage limit. */
	quotaBytes: number | undefined;
};

/** How much of the workspace's storage is taken, as text and a native progress bar. */
export function QuotaMeter({ usedBytes, quotaBytes }: Props) {
	const percent = usagePercent(usedBytes, quotaBytes);
	const full = quotaBytes !== undefined && percent >= 100;
	return (
		<div className="space-y-1.5">
			<p className="text-sm text-fg">
				{quotaBytes === undefined
					? `${formatBytes(usedBytes)} used, no storage limit`
					: `${formatBytes(usedBytes)} of ${formatBytes(quotaBytes)} used`}
			</p>
			{quotaBytes === undefined ? null : (
				<progress
					className={`h-2 w-full ${full ? "accent-danger" : "accent-brand-600"}`}
					aria-label="Storage used"
					value={percent}
					max={100}
				/>
			)}
			{full ? (
				<p className="text-sm text-danger">
					Storage is full. Delete files or upgrade your plan to upload more.
				</p>
			) : null}
		</div>
	);
}

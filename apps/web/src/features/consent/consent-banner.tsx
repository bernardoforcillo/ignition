import { analytics } from "~/lib/analytics";
import { useConsentStore } from "~/stores/consent";

/**
 * Asks once whether analytics may run. Shown only when analytics is configured and the visitor has
 * not chosen. Declining is as easy as accepting and the product works the same either way.
 */
export function ConsentBanner() {
	const decision = useConsentStore((state) => state.decision);
	const grant = useConsentStore((state) => state.grant);
	const deny = useConsentStore((state) => state.deny);

	if (!analytics.enabled || decision !== "unset") return null;

	return (
		<section
			aria-label="Analytics consent"
			className="fixed inset-x-0 bottom-0 z-50 border-t border-line bg-surface p-4 shadow-lg"
		>
			<div className="mx-auto flex max-w-3xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<p className="text-sm text-fg">
					We use privacy-friendly analytics to understand how the product is
					used and to fix errors. Nothing is collected unless you accept.
				</p>
				<div className="flex shrink-0 gap-2">
					<button
						type="button"
						onClick={deny}
						className="rounded-card border border-line px-3 py-1.5 text-sm font-medium text-fg hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500"
					>
						Decline
					</button>
					<button
						type="button"
						onClick={grant}
						className="rounded-card bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-2 focus-visible:outline-brand-500"
					>
						Accept analytics
					</button>
				</div>
			</div>
		</section>
	);
}

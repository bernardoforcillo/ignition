import { Button, Card } from "@ignition/components";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";

/**
 * Proves the cross-package wiring: renders real components imported from the `@ignition/components`
 * workspace package (not just plausible on paper). Also gives `motion` a second, app-level
 * usage beyond the one already inside the shared Button — an animated mount/exit transition
 * driven from this app's own state.
 */
export function ShowcasePanel() {
	const [expanded, setExpanded] = useState(false);

	return (
		<Card title="Shared component showcase">
			<p className="mb-3">
				<code>Button</code> and <code>Card</code> below are imported from{" "}
				<code>@ignition/components</code>, a separate workspace package.
			</p>

			<div className="flex flex-wrap items-center gap-3">
				<Button onClick={() => setExpanded((value) => !value)}>
					{expanded ? "Hide details" : "Show details"}
				</Button>
				<Button variant="secondary">Secondary action</Button>
			</div>

			<AnimatePresence>
				{expanded ? (
					<motion.div
						initial={{ opacity: 0, height: 0 }}
						animate={{ opacity: 1, height: "auto" }}
						exit={{ opacity: 0, height: 0 }}
						transition={{ duration: 0.2 }}
						className="mt-3 overflow-hidden rounded-card bg-brand-50 p-3 text-xs text-brand-700"
					>
						This panel animates in and out with <code>motion</code>, and the
						buttons above are the shared, Tailwind-styled{" "}
						<code>@ignition/components</code> <code>Button</code> component
						(which itself uses <code>motion</code> for its hover/tap
						micro-interaction).
					</motion.div>
				) : null}
			</AnimatePresence>
		</Card>
	);
}

import { type ReactNode, useEffect, useRef, useState } from "react";

import { nodeText } from "./node-text";

/** A fenced block with a copy button. The button's label changes politely for screen readers. */
export function CodeBlock({ children }: { children: ReactNode }) {
	const [copied, setCopied] = useState(false);
	const timer = useRef<number | undefined>(undefined);
	useEffect(() => () => window.clearTimeout(timer.current), []);

	const copy = async () => {
		try {
			await navigator.clipboard.writeText(
				nodeText(children).replace(/\n$/, ""),
			);
			setCopied(true);
			window.clearTimeout(timer.current);
			timer.current = window.setTimeout(() => setCopied(false), 2000);
		} catch {
			// Clipboard blocked (insecure origin, permissions): the text stays selectable.
		}
	};

	return (
		<div className="relative my-4">
			<pre
				// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region must be reachable by keyboard
				tabIndex={0}
				className="overflow-x-auto rounded-card border border-line bg-surface-muted p-4 pr-16 font-mono text-sm text-fg focus-visible:outline-2 focus-visible:outline-brand-500"
			>
				{children}
			</pre>
			<button
				type="button"
				onClick={copy}
				className="absolute right-2 top-2 rounded-md border border-line bg-surface px-2 py-1 text-xs font-medium text-fg hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500"
			>
				{copied ? "Copied" : "Copy"}
				<span className="sr-only"> code to clipboard</span>
			</button>
		</div>
	);
}

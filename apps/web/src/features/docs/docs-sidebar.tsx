import { Link } from "@tanstack/react-router";

import { DOCS } from "./manifest";

type Props = { onNavigate: () => void };

export function DocsSidebar({ onNavigate }: Props) {
	return (
		<nav aria-label="Documentation">
			<ul className="space-y-1">
				{DOCS.map((doc) => (
					<li key={doc.slug}>
						<Link
							to="/docs/$slug"
							params={{ slug: doc.slug }}
							onClick={onNavigate}
							className="block rounded-card px-3 py-1.5 text-sm text-fg-muted hover:bg-surface-muted hover:text-fg focus-visible:outline-2 focus-visible:outline-brand-500 aria-[current=page]:bg-brand-50 aria-[current=page]:font-medium aria-[current=page]:text-brand-700 dark:aria-[current=page]:bg-surface-muted dark:aria-[current=page]:text-brand-300"
						>
							{doc.title}
						</Link>
					</li>
				))}
			</ul>
		</nav>
	);
}

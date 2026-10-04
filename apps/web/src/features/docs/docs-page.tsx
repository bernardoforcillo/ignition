import { Alert, Skeleton } from "@ignition/components";
import { useQuery } from "@tanstack/react-query";
import { Link, useLocation, useParams } from "@tanstack/react-router";
import { useEffect, useMemo } from "react";

import { analytics } from "~/lib/analytics";
import { useDocumentMeta } from "~/lib/document-meta";

import { extractHeadings } from "./headings";
import { loadDoc } from "./load-doc";
import { findDoc, neighbours } from "./manifest";
import { Markdown } from "./markdown";

const pagerLink =
	"flex flex-col rounded-card border border-line bg-surface p-4 hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-brand-500";

function UnknownPage() {
	useDocumentMeta({
		title: "Page not found | Ignition docs",
		description: "This documentation page does not exist.",
	});
	return (
		<div className="space-y-3">
			<h1 className="text-2xl font-semibold text-fg">Page not found</h1>
			<p className="text-fg-muted">
				There is no documentation page at this address.
			</p>
			<Link
				to="/docs/$slug"
				params={{ slug: "overview" }}
				className="font-medium text-brand-600 underline"
			>
				Go to the overview
			</Link>
		</div>
	);
}

function DocArticle({ slug }: { slug: string }) {
	const entry = findDoc(slug);
	const hash = useLocation({ select: (location) => location.hash });
	const source = useQuery({
		queryKey: ["docs", slug],
		queryFn: () => loadDoc(slug),
		staleTime: Number.POSITIVE_INFINITY,
	});
	const headings = useMemo(
		() => (source.data ? extractHeadings(source.data) : []),
		[source.data],
	);

	useDocumentMeta({
		title: `${entry?.title ?? "Docs"} | Ignition docs`,
		description: entry?.summary ?? "",
	});

	useEffect(() => {
		analytics.track("docs_page_viewed", { location: "docs", slug });
	}, [slug]);

	const loaded = source.data !== undefined && source.data !== null;
	useEffect(() => {
		if (!loaded) return;
		const target = hash ? document.getElementById(hash) : null;
		if (target) target.scrollIntoView();
		else window.scrollTo(0, 0);
	}, [loaded, hash]);

	const { previous, next } = neighbours(slug);

	if (source.isPending) {
		return (
			<div className="space-y-4">
				<Skeleton className="h-9 w-64" />
				<Skeleton className="h-40 w-full" />
			</div>
		);
	}
	if (source.isError || source.data == null) {
		return (
			<Alert tone="danger">This page could not be loaded. Try again.</Alert>
		);
	}

	return (
		<article>
			{headings.length > 1 ? (
				<nav
					aria-label="On this page"
					className="mb-6 rounded-card border border-line bg-surface-muted p-4"
				>
					<p className="text-sm font-semibold text-fg">On this page</p>
					<ul className="mt-2 space-y-1 text-sm">
						{headings
							.filter((heading) => heading.level === 2)
							.map((heading) => (
								<li key={heading.id}>
									<a
										href={`#${heading.id}`}
										className="text-brand-600 hover:underline focus-visible:outline-2 focus-visible:outline-brand-500"
									>
										{heading.text}
									</a>
								</li>
							))}
					</ul>
				</nav>
			) : null}
			<Markdown source={source.data} />
			<nav
				aria-label="Pagination"
				className="mt-12 grid gap-4 border-t border-line pt-6 sm:grid-cols-2"
			>
				{previous ? (
					<Link
						to="/docs/$slug"
						params={{ slug: previous.slug }}
						rel="prev"
						className={pagerLink}
					>
						<span className="text-xs text-fg-muted">Previous</span>
						<span className="font-medium text-fg">{previous.title}</span>
					</Link>
				) : (
					<span />
				)}
				{next ? (
					<Link
						to="/docs/$slug"
						params={{ slug: next.slug }}
						rel="next"
						className={`${pagerLink} sm:items-end sm:text-right`}
					>
						<span className="text-xs text-fg-muted">Next</span>
						<span className="font-medium text-fg">{next.title}</span>
					</Link>
				) : null}
			</nav>
		</article>
	);
}

export function DocsPage() {
	const { slug } = useParams({ strict: false });
	if (!slug || !findDoc(slug)) return <UnknownPage />;
	return <DocArticle key={slug} slug={slug} />;
}

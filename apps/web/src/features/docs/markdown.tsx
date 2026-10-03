import { Link } from "@tanstack/react-router";
import type { ComponentProps, ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { CodeBlock } from "./code-block";
import { headingId } from "./headings";
import { classifyLink } from "./links";
import { nodeText } from "./node-text";

const linkClass =
	"font-medium text-brand-600 underline underline-offset-2 hover:text-brand-700 focus-visible:outline-2 focus-visible:outline-brand-500";

function Heading({ level, children }: { level: 2 | 3; children?: ReactNode }) {
	const id = headingId(nodeText(children));
	const Tag = level === 2 ? "h2" : "h3";
	return (
		<Tag
			id={id}
			className={
				level === 2
					? "mt-10 scroll-mt-20 border-b border-line pb-2 text-2xl font-semibold text-fg"
					: "mt-8 scroll-mt-20 text-lg font-semibold text-fg"
			}
		>
			<a href={`#${id}`} className="hover:underline">
				{children}
			</a>
		</Tag>
	);
}

function DocLinkView({ href, children }: ComponentProps<"a">) {
	const link = classifyLink(href);
	if (link.kind === "docs") {
		return (
			<Link
				to="/docs/$slug"
				params={{ slug: link.slug }}
				hash={link.hash ?? undefined}
				className={linkClass}
			>
				{children}
			</Link>
		);
	}
	if (link.kind === "external") {
		return (
			<a href={link.href} className={linkClass} rel="noopener noreferrer">
				{children}
			</a>
		);
	}
	return <span>{children}</span>;
}

const components: Components = {
	h1: ({ children }) => (
		<h1 className="text-3xl font-semibold tracking-tight text-fg">
			{children}
		</h1>
	),
	h2: ({ children }) => <Heading level={2}>{children}</Heading>,
	h3: ({ children }) => <Heading level={3}>{children}</Heading>,
	p: ({ children }) => <p className="mt-4 leading-7 text-fg">{children}</p>,
	ul: ({ children }) => (
		<ul className="mt-4 list-disc space-y-1.5 pl-6 text-fg">{children}</ul>
	),
	ol: ({ children }) => (
		<ol className="mt-4 list-decimal space-y-1.5 pl-6 text-fg">{children}</ol>
	),
	a: DocLinkView,
	blockquote: ({ children }) => (
		<blockquote className="mt-4 rounded-card border-l-4 border-brand-500 bg-surface-muted px-4 py-1 text-fg-muted">
			{children}
		</blockquote>
	),
	pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
	code: ({ children, className }) => {
		// A fenced block's text ends with a newline; inline code never does.
		const block =
			/language-/.test(className ?? "") || /\n$/.test(nodeText(children));
		return block ? (
			<code className={className}>{children}</code>
		) : (
			<code className="rounded bg-surface-muted px-1.5 py-0.5 font-mono text-[0.9em] text-fg">
				{children}
			</code>
		);
	},
	table: ({ children }) => (
		<div className="mt-4 overflow-x-auto rounded-card border border-line">
			<table className="w-full border-collapse text-left text-sm">
				{children}
			</table>
		</div>
	),
	thead: ({ children }) => (
		<thead className="bg-surface-muted">{children}</thead>
	),
	th: ({ children }) => (
		<th className="border-b border-line px-3 py-2 font-semibold text-fg">
			{children}
		</th>
	),
	td: ({ children }) => (
		<td className="border-b border-line px-3 py-2 align-top text-fg">
			{children}
		</td>
	),
	hr: () => <hr className="my-8 border-line" />,
};

/** Renders trusted-but-untrusted-by-construction Markdown: raw HTML is dropped, links are vetted. */
export function Markdown({ source }: { source: string }) {
	return (
		<ReactMarkdown remarkPlugins={[remarkGfm]} components={components} skipHtml>
			{source}
		</ReactMarkdown>
	);
}

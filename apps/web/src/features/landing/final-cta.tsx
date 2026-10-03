import { PrimaryCta } from "~/features/site-chrome";

export function FinalCta() {
	return (
		<section aria-labelledby="cta-title">
			<div className="mx-auto max-w-3xl space-y-5 px-4 py-16 text-center">
				<h2
					id="cta-title"
					className="text-3xl font-semibold tracking-tight text-fg"
				>
					Ready to build on a solid base?
				</h2>
				<p className="text-fg-muted">
					Create a free account and have a workspace running in a minute.
				</p>
				<PrimaryCta label="Create your free account" />
			</div>
		</section>
	);
}

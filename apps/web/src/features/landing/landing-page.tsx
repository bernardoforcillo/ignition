import {
	MAIN_CONTENT_ID,
	SiteFooter,
	SiteHeader,
} from "~/features/site-chrome";
import { useDocumentMeta } from "~/lib/document-meta";

import { FaqSection } from "./faq-section";
import { FinalCta } from "./final-cta";
import { Hero } from "./hero";
import { Highlights } from "./highlights";
import { PricingSection } from "./pricing-section";

export function LandingPage() {
	useDocumentMeta({
		title: "Ignition: launch your SaaS with the hard parts built",
		description:
			"A SaaS starter with accounts, workspaces and roles, plans and metered limits, Stripe billing and GDPR-ready data handling.",
	});
	return (
		<>
			<SiteHeader />
			<main id={MAIN_CONTENT_ID} tabIndex={-1} className="outline-none">
				<Hero />
				<Highlights />
				<PricingSection />
				<FaqSection />
				<FinalCta />
			</main>
			<SiteFooter />
		</>
	);
}

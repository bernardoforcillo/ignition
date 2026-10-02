import type { ReactNode } from "react";
import {
	Body,
	Column,
	Container,
	Head,
	Heading,
	Html,
	Img,
	Link,
	Preview,
	Row,
	Section,
	Tailwind,
	Text,
} from "react-email";

import { assetBaseUrl } from "./assets";
import { brand } from "./brand";
import { barebonesBoxedTailwindConfig } from "./theme";
import { BarebonesFonts } from "./theme-fonts";

export interface ShellProps {
	companyName: string;
	preview: string;
	title: string;
	children: ReactNode;
}

/**
 * The Barebone layout (header, centered card, footer) for the templates this product adds on top
 * of the demo set, so they match `activation.tsx` and friends.
 */
export function Shell({ companyName, preview, title, children }: ShellProps) {
	return (
		<Tailwind config={barebonesBoxedTailwindConfig}>
			<Html>
				<Head>
					<BarebonesFonts />
				</Head>

				<Body className="bg-bg-2 m-0 text-center font-sans">
					<Preview>{preview}</Preview>
					<Container className="mobile:mt-0 mx-auto mt-8 w-full max-w-[640px]">
						<Section className="bg-bg mobile:px-2 px-6 py-4">
							<Section className="mb-3 px-6">
								<Row>
									<Column className="w-1/2 py-[7px] align-middle">
										<Img
											src={`${assetBaseUrl}/static/shared/logo-black.png`}
											alt=""
											width={23}
											className="block"
										/>
									</Column>
									<Column align="right" className="w-1/2 py-[7px] align-middle">
										<Text className="font-13 m-0 text-right font-sans">
											<span className="text-fg-3">{companyName}</span>
										</Text>
									</Column>
								</Row>
							</Section>

							<Section className="bg-bg-2 mobile:px-6 mobile:py-12 rounded-[8px] px-[40px] py-[64px] text-center">
								<Heading as="h1" className="font-28 text-fg m-0 mb-3 font-sans">
									{title}
								</Heading>
								{children}
							</Section>

							<Section className="bg-bg">
								<Row>
									<Column className="px-6 py-10 text-center">
										<Text className="font-13 text-fg-3 mx-auto mt-0 mb-8 max-w-[280px] text-center font-sans">
											{brand.tagline}
										</Text>
										<Text className="font-11 text-fg-3 mt-4 mb-5 text-center font-sans">
											{brand.addressLines[0]}
											<br />
											{brand.addressLines[1]}
										</Text>
										<Text className="font-11 text-fg-3 m-0 text-center font-sans">
											<Link href={brand.unsubscribeUrl} className="text-fg-3">
												Manage notifications
											</Link>
										</Text>
									</Column>
								</Row>
							</Section>
						</Section>
					</Container>
				</Body>
			</Html>
		</Tailwind>
	);
}

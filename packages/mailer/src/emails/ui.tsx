import { Button as EmailButton, Heading, Text } from "@react-email/components";
import type { ReactNode } from "react";

import { brand } from "./layout";

export function Title({ children }: { children: ReactNode }) {
	return (
		<Heading
			as="h1"
			style={{ color: brand.text, fontSize: "22px", margin: "24px 0 8px" }}
		>
			{children}
		</Heading>
	);
}

export function Paragraph({ children }: { children: ReactNode }) {
	return (
		<Text style={{ color: brand.text, fontSize: "15px", lineHeight: "24px" }}>
			{children}
		</Text>
	);
}

export function ActionButton({
	href,
	children,
}: {
	href: string;
	children: ReactNode;
}) {
	return (
		<EmailButton
			href={href}
			style={{
				backgroundColor: brand.primary,
				borderRadius: "8px",
				color: "#ffffff",
				display: "inline-block",
				fontSize: "15px",
				fontWeight: 600,
				padding: "12px 20px",
				textDecoration: "none",
			}}
		>
			{children}
		</EmailButton>
	);
}

export function Fallback({ link }: { link: string }) {
	return (
		<Text style={{ color: brand.muted, fontSize: "13px", lineHeight: "20px" }}>
			Button not working? Paste this link into your browser: {link}
		</Text>
	);
}

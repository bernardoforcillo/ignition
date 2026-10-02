import {
	Body,
	Container,
	Head,
	Hr,
	Html,
	Preview,
	Section,
	Text,
} from "@react-email/components";
import type { ReactNode } from "react";

/** Brand tokens mirrored from `@ignition/components/theme.css` (email clients ignore CSS variables). */
export const brand = {
	primary: "#4f46e5",
	text: "#1f2937",
	muted: "#6b7280",
	background: "#f9fafb",
	radius: "12px",
} as const;

const styles = {
	body: {
		backgroundColor: brand.background,
		fontFamily:
			"Inter, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
		margin: 0,
		padding: "24px 0",
	},
	container: {
		backgroundColor: "#ffffff",
		borderRadius: brand.radius,
		margin: "0 auto",
		maxWidth: "520px",
		padding: "32px",
	},
	brand: { color: brand.primary, fontSize: "18px", fontWeight: 700, margin: 0 },
	hr: { borderColor: "#e5e7eb", margin: "24px 0" },
	footer: {
		color: brand.muted,
		fontSize: "12px",
		lineHeight: "18px",
		margin: 0,
	},
} as const;

export interface EmailLayoutProps {
	/** Inbox preview text shown next to the subject. */
	preview: string;
	children: ReactNode;
}

/** Shared chrome: brand header, content card and footer. Every template renders inside it. */
export function EmailLayout({ preview, children }: EmailLayoutProps) {
	return (
		<Html lang="en">
			<Head />
			<Preview>{preview}</Preview>
			<Body style={styles.body}>
				<Container style={styles.container}>
					<Text style={styles.brand}>Ignition</Text>
					<Section>{children}</Section>
					<Hr style={styles.hr} />
					<Text style={styles.footer}>
						You are receiving this email because of activity on your Ignition
						account. If this wasn't you, you can safely ignore it.
					</Text>
				</Container>
			</Body>
		</Html>
	);
}

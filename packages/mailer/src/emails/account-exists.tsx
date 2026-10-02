import { EmailLayout } from "./layout";
import { Paragraph, Title } from "./ui";

export interface AccountExistsProps {
	loginUrl: string;
}

/**
 * Sent instead of a verification link when someone signs up with an address that already has an
 * account, so the sign-up endpoint never reveals whether an address is registered.
 */
export default function AccountExists({ loginUrl }: AccountExistsProps) {
	return (
		<EmailLayout preview="Someone tried to sign up with your email address.">
			<Title>You already have an account</Title>
			<Paragraph>
				Someone tried to create an account with this address, but one already
				exists. If it was you, sign in at {loginUrl}. If it wasn't, no action is
				needed.
			</Paragraph>
		</EmailLayout>
	);
}

AccountExists.PreviewProps = {
	loginUrl: "https://app.example.com/login",
} satisfies AccountExistsProps;

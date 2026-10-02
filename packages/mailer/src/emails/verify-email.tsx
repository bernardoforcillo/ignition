import { EmailLayout } from "./layout";
import { ActionButton, Fallback, Paragraph, Title } from "./ui";

export interface VerifyEmailProps {
	link: string;
}

export default function VerifyEmail({ link }: VerifyEmailProps) {
	return (
		<EmailLayout preview="Confirm your email address to finish signing up.">
			<Title>Confirm your email</Title>
			<Paragraph>
				Thanks for signing up. Confirm your address to activate your account.
			</Paragraph>
			<ActionButton href={link}>Confirm email</ActionButton>
			<Fallback link={link} />
		</EmailLayout>
	);
}

VerifyEmail.PreviewProps = {
	link: "https://app.example.com/verify?token=preview",
} satisfies VerifyEmailProps;

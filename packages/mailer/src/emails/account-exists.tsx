import { Button, Section, Text } from "react-email";

import { Shell } from "./shell";

export interface AccountExistsProps {
	companyName: string;
	url: string;
}

/**
 * Sent instead of a verification link when someone signs up with an address that already has an
 * account, so the sign-up endpoint never reveals whether an address is registered.
 */
export default function AccountExists({
	companyName,
	url,
}: AccountExistsProps) {
	return (
		<Shell
			companyName={companyName}
			preview="Someone tried to sign up with your email address."
			title="You already have an account"
		>
			<Text className="font-16 text-fg-2 mx-auto mt-0 mb-8 max-w-[380px] text-center font-sans">
				Someone tried to create a {companyName} account with this address, but
				one already exists. If it was you, sign in below. If it wasn&apos;t, no
				action is needed.
			</Text>
			<Section className="text-center">
				<Button
					href={url}
					className="bg-fg font-16 text-fg-inverted inline-block rounded-lg px-7 py-4 text-center font-sans leading-6"
				>
					Sign in
				</Button>
			</Section>
		</Shell>
	);
}

AccountExists.PreviewProps = {
	companyName: "Barebones",
	url: "https://example.com/login",
} satisfies AccountExistsProps;

import { Button, Section, Text } from "react-email";

import { Shell } from "./shell";

export interface TrialEndingProps {
	companyName: string;
	workspaceName: string;
	endsOn: string;
	url: string;
}

export default function TrialEnding({
	companyName,
	workspaceName,
	endsOn,
	url,
}: TrialEndingProps) {
	return (
		<Shell
			companyName={companyName}
			preview={`The ${workspaceName} trial ends on ${endsOn}.`}
			title="Your trial is ending soon"
		>
			<Text className="font-16 text-fg-2 mx-auto mt-0 mb-8 max-w-[380px] text-center font-sans">
				The {companyName} trial of the {workspaceName} workspace ends on{" "}
				{endsOn}. Review your billing details to keep your plan without
				interruption.
			</Text>
			<Section className="text-center">
				<Button
					href={url}
					className="bg-fg font-16 text-fg-inverted inline-block rounded-lg px-7 py-4 text-center font-sans leading-6"
				>
					Review billing
				</Button>
			</Section>
		</Shell>
	);
}

TrialEnding.PreviewProps = {
	companyName: "Barebones",
	workspaceName: "Acme",
	endsOn: "12 October 2026",
	url: "https://example.com/app/billing",
} satisfies TrialEndingProps;

import { Button, Section, Text } from "react-email";

import { Shell } from "./shell";

export interface WorkspaceInvitationProps {
	companyName: string;
	workspaceName: string;
	url: string;
}

export default function WorkspaceInvitation({
	companyName,
	workspaceName,
	url,
}: WorkspaceInvitationProps) {
	return (
		<Shell
			companyName={companyName}
			preview={`You have been invited to ${workspaceName}.`}
			title={`Join ${workspaceName}`}
		>
			<Text className="font-16 text-fg-2 mx-auto mt-0 mb-8 max-w-[380px] text-center font-sans">
				You have been invited to collaborate in the {workspaceName} workspace on{" "}
				{companyName}.
			</Text>
			<Section className="text-center">
				<Button
					href={url}
					className="bg-fg font-16 text-fg-inverted inline-block rounded-lg px-7 py-4 text-center font-sans leading-6"
				>
					Accept invitation
				</Button>
			</Section>
		</Shell>
	);
}

WorkspaceInvitation.PreviewProps = {
	companyName: "Barebones",
	workspaceName: "Acme",
	url: "https://example.com/invite?token=preview",
} satisfies WorkspaceInvitationProps;

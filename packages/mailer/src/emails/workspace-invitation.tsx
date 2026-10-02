import { EmailLayout } from "./layout";
import { ActionButton, Fallback, Paragraph, Title } from "./ui";

export interface WorkspaceInvitationProps {
	workspaceName: string;
	link: string;
}

export default function WorkspaceInvitation({
	workspaceName,
	link,
}: WorkspaceInvitationProps) {
	return (
		<EmailLayout preview={`You have been invited to ${workspaceName}.`}>
			<Title>Join {workspaceName}</Title>
			<Paragraph>
				You have been invited to collaborate in the {workspaceName} workspace.
			</Paragraph>
			<ActionButton href={link}>Accept invitation</ActionButton>
			<Fallback link={link} />
		</EmailLayout>
	);
}

WorkspaceInvitation.PreviewProps = {
	workspaceName: "Acme",
	link: "https://app.example.com/invite?token=preview",
} satisfies WorkspaceInvitationProps;

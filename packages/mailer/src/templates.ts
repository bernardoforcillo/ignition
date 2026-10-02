import type { ComponentType } from "react";

import AccountExists from "./emails/account-exists";
import VerifyEmail from "./emails/verify-email";
import WorkspaceInvitation from "./emails/workspace-invitation";

/** One transactional email: its component and how its subject is built. */
export interface TemplateDefinition<P extends object> {
	component: ComponentType<P>;
	/** Builds the subject line from the same props the body receives. */
	subject: (props: P) => string;
	/** Prop names, in the order they appear in the Go template data (PascalCased there). */
	variables: readonly (keyof P & string)[];
}

function define<P extends object>(
	definition: TemplateDefinition<P>,
): TemplateDefinition<P> {
	return definition;
}

/**
 * Every email the product sends. Add a template here and in `emails/`, then run
 * `pnpm --filter @ignition/mailer export` to regenerate the HTML the Go mailer embeds.
 */
export const templates = {
	"verify-email": define({
		component: VerifyEmail,
		subject: () => "Confirm your email address",
		variables: ["link"],
	}),
	"account-exists": define({
		component: AccountExists,
		subject: () => "You already have an account",
		variables: ["loginUrl"],
	}),
	"workspace-invitation": define({
		component: WorkspaceInvitation,
		subject: ({ workspaceName }) => `Join ${workspaceName} on Ignition`,
		variables: ["workspaceName", "link"],
	}),
} as const;

export type TemplateName = keyof typeof templates;
export type TemplateProps<N extends TemplateName> =
	(typeof templates)[N] extends TemplateDefinition<infer P> ? P : never;

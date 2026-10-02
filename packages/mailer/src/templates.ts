import type { ComponentType } from "react";

import AccountExists from "./emails/account-exists";
import Activation from "./emails/activation";
import FeatureAnnouncement from "./emails/feature-announcement";
import PasswordReset from "./emails/password-reset";
import ProductUpdate from "./emails/product-update";
import SubscriptionConfirmation from "./emails/subscription-confirmation";
import SubscriptionUpdate from "./emails/subscription-update";
import TextOnly from "./emails/text-only";
import Welcome from "./emails/welcome";
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

/** Variables the Go mailer fills in itself from its config; callers never pass them. */
export const ambientVariables = ["companyName"] as const;

const subscriptionVariables = [
	"companyName",
	"url",
	"userName",
	"planName",
	"planPrice",
	"cycleLabel",
	"nextBillingDate",
] as const;

/**
 * Every email the product sends. Add a template here and in `emails/`, then run
 * `pnpm --filter @ignition/mailer export` to regenerate the HTML the Go mailer embeds.
 *
 * `activation`, `welcome`, `password-reset`, `subscription-*`, `feature-announcement`,
 * `product-update` and `text-only` are the react.email "Barebone" collection
 * (https://demo.react.email/preview/01-Barebone/welcome).
 */
export const templates = {
	activation: define({
		component: Activation,
		subject: () => "Confirm your email address",
		variables: ["companyName", "url"],
	}),
	welcome: define({
		component: Welcome,
		subject: ({ companyName }) => `Welcome to ${companyName}`,
		variables: ["companyName", "url"],
	}),
	"password-reset": define({
		component: PasswordReset,
		subject: () => "Reset your password",
		variables: ["companyName", "url"],
	}),
	"subscription-confirmation": define({
		component: SubscriptionConfirmation,
		subject: ({ planName }) => `Your ${planName} subscription is confirmed`,
		variables: subscriptionVariables,
	}),
	"subscription-update": define({
		component: SubscriptionUpdate,
		subject: ({ planName }) => `Your ${planName} subscription was updated`,
		variables: subscriptionVariables,
	}),
	"feature-announcement": define({
		component: FeatureAnnouncement,
		subject: ({ companyName }) => `Release notes from ${companyName}`,
		variables: ["companyName", "url"],
	}),
	"product-update": define({
		component: ProductUpdate,
		subject: ({ companyName }) => `What's new at ${companyName}`,
		variables: ["companyName", "url"],
	}),
	"text-only": define({
		component: TextOnly,
		subject: ({ companyName }) => `A note from ${companyName}`,
		variables: ["companyName", "url"],
	}),
	"account-exists": define({
		component: AccountExists,
		subject: () => "You already have an account",
		variables: ["companyName", "url"],
	}),
	"workspace-invitation": define({
		component: WorkspaceInvitation,
		subject: ({ workspaceName }) => `Join ${workspaceName}`,
		variables: ["companyName", "workspaceName", "url"],
	}),
} as const;

export type TemplateName = keyof typeof templates;
export type TemplateProps<N extends TemplateName> =
	(typeof templates)[N] extends TemplateDefinition<infer P> ? P : never;

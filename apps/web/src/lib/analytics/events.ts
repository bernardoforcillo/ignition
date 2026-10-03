/**
 * Every product event the web app sends, with its properties. `track` only accepts names and
 * property shapes declared here, so the vocabulary stays small and reviewable.
 *
 * Conventions: `object_verb` in snake_case, past tense; always a `location` naming the surface.
 * Properties are primitives and never personal data (no emails, names, tokens or free text):
 * send an id, a category, a length or a boolean instead of the raw value.
 */
export interface AnalyticsEvents {
	signup_submitted: { location: "signup" };
	email_verified: { location: "verify_email" };
	login_succeeded: { location: "login" };
	password_reset_requested: { location: "forgot_password" };
	password_reset_completed: { location: "reset_password" };
	workspace_created: { location: "onboarding" };
	invitation_sent: { location: "members"; role_key: string };
	invitation_accepted: { location: "invite_accept" };
	checkout_started: { location: "billing"; kind: string; target_id: string };
	billing_portal_opened: { location: "billing" };
	data_exported: { location: "settings" };
	account_deleted: { location: "settings" };
	pricing_cta_clicked: { location: "landing"; plan_id: string };
	docs_page_viewed: { location: "docs"; slug: string };
}

export type AnalyticsEventName = keyof AnalyticsEvents;

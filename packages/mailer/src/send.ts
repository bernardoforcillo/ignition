import { render } from "@react-email/render";
import { type ComponentType, createElement } from "react";
import { Resend } from "resend";

import { plainTextOptions } from "./plain-text";
import { type TemplateName, type TemplateProps, templates } from "./templates";

export interface MailerConfig {
	apiKey: string;
	/** Default sender, e.g. `Ignition <hello@example.com>`. */
	from: string;
	replyTo?: string;
}

export interface SendOptions {
	to: string | string[];
	/** Overrides the default sender for this message. */
	from?: string;
	/** Resend de-duplicates retries carrying the same key. */
	idempotencyKey?: string;
}

/**
 * Renders a template and sends it through Resend. For Node/Edge apps; the Go services use the
 * static export and `go-packages/mailer` instead.
 */
export function createMailer(config: MailerConfig) {
	const resend = new Resend(config.apiKey);

	return {
		async send<N extends TemplateName>(
			name: N,
			props: TemplateProps<N>,
			options: SendOptions,
		): Promise<{ id: string }> {
			const definition = templates[name];
			// The registry is typed per template; widen once here so one code path serves all.
			const component = definition.component as ComponentType<TemplateProps<N>>;
			const element = createElement(component, props);
			const [html, text] = await Promise.all([
				render(element),
				render(element, plainTextOptions),
			]);

			const { data, error } = await resend.emails.send(
				{
					from: options.from ?? config.from,
					to: options.to,
					replyTo: config.replyTo,
					subject: (definition.subject as (p: TemplateProps<N>) => string)(
						props,
					),
					html,
					text,
				},
				options.idempotencyKey
					? { idempotencyKey: options.idempotencyKey }
					: undefined,
			);
			if (error || !data) {
				throw new Error(`resend: ${error?.message ?? "no response"}`);
			}
			return { id: data.id };
		},
	};
}

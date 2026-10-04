import { expect } from "@playwright/test";

import { env } from "../harness/env";
import type { Mail } from "../harness/fake-resend";

export type { Mail };

async function mailsTo(address: string): Promise<Mail[]> {
	const res = await fetch(
		`${env.resendUrl}/__mail?to=${encodeURIComponent(address)}`,
	);
	return (await res.json()) as Mail[];
}

/** Every message the fake Resend received for an address, oldest first. */
export const allMail = mailsTo;

/** Waits for a message to `address` whose subject matches, and returns the newest such one. */
export async function waitForMail(
	address: string,
	subject: RegExp = /./,
): Promise<Mail> {
	let found: Mail | undefined;
	await expect
		.poll(
			async () => {
				found = (await mailsTo(address))
					.filter((m) => subject.test(m.subject))
					.at(-1);
				return found !== undefined;
			},
			{ message: `mail to ${address} matching ${subject}`, timeout: 15_000 },
		)
		.toBe(true);
	return found as Mail;
}

/**
 * The first link in the text body that points at `path` of the web app (`/verify-email`,
 * `/reset-password`, `/invite/accept`). Returned as an absolute URL.
 */
export function linkTo(mail: Mail, path: string): string {
	const escaped = path.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
	const match = mail.text.match(
		new RegExp(`https?://[^\\s"'<>]*${escaped}\\?[^\\s"'<>]+`),
	);
	if (!match) {
		throw new Error(
			`no ${path} link in "${mail.subject}": ${mail.text.slice(0, 400)}`,
		);
	}
	return match[0].replaceAll("&amp;", "&");
}

export const tokenOf = (link: string): string => {
	const token = new URL(link).searchParams.get("token");
	if (!token) throw new Error(`no token in ${link}`);
	return token;
};

/** Waits for a message to `address` that carries a link to `path` and returns the newest such link. */
export async function waitForLink(
	address: string,
	path: string,
): Promise<string> {
	let link: string | undefined;
	await expect
		.poll(
			async () => {
				const mail = (await mailsTo(address))
					.filter((m) => m.text.includes(`${path}?`))
					.at(-1);
				link = mail ? linkTo(mail, path) : undefined;
				return link !== undefined;
			},
			{ message: `a ${path} link mailed to ${address}`, timeout: 15_000 },
		)
		.toBe(true);
	return link as string;
}

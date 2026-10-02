import { type Fake, json, startServer } from "./http.js";

export interface Mail {
	id: string;
	to: string[];
	subject: string;
	html: string;
	text: string;
}

/**
 * Stands in for Resend's `POST /emails` (the gateway reaches it through RESEND_BASE_URL) and keeps
 * every message. `GET /__mail?to=<address>` lists the ones sent to an address, oldest first.
 */
export function startFakeResend(apiKey: string): Promise<Fake> {
	const mails: Mail[] = [];
	return startServer((req, res, body) => {
		const url = new URL(req.url ?? "/", "http://fake");
		if (req.method === "POST" && url.pathname === "/emails") {
			if (req.headers.authorization !== `Bearer ${apiKey}`) {
				return json(res, 401, { name: "unauthorized", message: "bad key" });
			}
			const sent = JSON.parse(body.toString("utf8")) as Omit<Mail, "id">;
			const mail: Mail = {
				id: `mail_${mails.length + 1}`,
				to: sent.to.map((address) => address.toLowerCase()),
				subject: sent.subject,
				html: sent.html ?? "",
				text: sent.text ?? "",
			};
			mails.push(mail);
			return json(res, 200, { id: mail.id });
		}
		if (req.method === "GET" && url.pathname === "/__mail") {
			const to = url.searchParams.get("to")?.toLowerCase();
			return json(
				res,
				200,
				mails.filter((mail) => !to || mail.to.includes(to)),
			);
		}
		json(res, 404, { message: "not found" });
	});
}

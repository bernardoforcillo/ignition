import { gunzipSync } from "node:zlib";

import { type Fake, json, startServer } from "./http.js";

export interface Capture {
	method: string;
	url: string;
	/** The request body, gunzipped when it was compressed. */
	body: string;
}

/**
 * A PostHog ingestion host that accepts anything and remembers it (`GET /__reset` forgets). posthog-js posts to it from the
 * web origin, so it answers CORS preflights. `GET /__captures` lists every request seen, URL and
 * (decompressed) body included, so a test can assert on the whole wire, not just on events.
 */
export function startFakePostHog(): Promise<Fake> {
	const captures: Capture[] = [];
	return startServer((req, res, raw) => {
		res.setHeader("access-control-allow-origin", req.headers.origin ?? "*");
		res.setHeader("access-control-allow-headers", "*");
		res.setHeader("access-control-allow-methods", "GET,POST,OPTIONS");
		const url = new URL(req.url ?? "/", "http://fake");
		if (req.method === "OPTIONS") {
			res.writeHead(204);
			return void res.end();
		}
		if (url.pathname === "/__captures") return json(res, 200, captures);
		if (url.pathname === "/__reset") {
			captures.length = 0;
			return json(res, 200, {});
		}
		captures.push({
			method: req.method ?? "GET",
			url: `${url.pathname}${url.search}`,
			body: decodeBody(raw),
		});
		const remote = {
			token: "",
			supportedCompression: ["gzip", "gzip-js"],
			hasFeatureFlags: false,
			captureDeadClicks: false,
			capturePerformance: false,
			autocapture_opt_out: false,
			autocaptureExceptions: false,
			analytics: { endpoint: "/i/v0/e/" },
			elementsChainAsString: true,
			sessionRecording: false,
			heatmaps: false,
			surveys: false,
			defaultIdentifiedOnly: true,
			siteApps: [],
		};
		if (/^\/array\/[^/]+\/config\.js$/.test(url.pathname)) {
			const key = url.pathname.split("/")[2];
			res.writeHead(200, { "content-type": "application/javascript" });
			return void res.end(
				`window._POSTHOG_REMOTE_CONFIG=window._POSTHOG_REMOTE_CONFIG||{};window._POSTHOG_REMOTE_CONFIG[${JSON.stringify(key)}]={config:${JSON.stringify(remote)},siteApps:[]};`,
			);
		}
		if (/^\/array\/[^/]+\/config$/.test(url.pathname))
			return json(res, 200, remote);
		if (url.pathname.startsWith("/static/")) {
			res.writeHead(404);
			return void res.end();
		}
		json(res, 200, {
			status: 1,
			featureFlags: {},
			featureFlagPayloads: {},
			flags: {},
			errorsWhileComputingFlags: false,
		});
	});
}

/**
 * posthog-js posts gzip, or a form `data=<base64 JSON>` (`compression=base64`). Returns the readable
 * JSON, followed by the raw text, so an assertion sees what the vendor would see either way.
 */
function decodeBody(raw: Buffer): string {
	let text: string;
	try {
		text = gunzipSync(raw).toString("utf8");
	} catch {
		text = raw.toString("utf8");
	}
	const form = /^data=([^&]*)/.exec(text);
	if (!form?.[1]) return text;
	try {
		const decoded = Buffer.from(decodeURIComponent(form[1]), "base64").toString(
			"utf8",
		);
		return `${decoded}\n${text}`;
	} catch {
		return text;
	}
}

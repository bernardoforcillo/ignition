import { createHash, createHmac, timingSafeEqual } from "node:crypto";

import { type Fake, json, startServer } from "./http.js";

export interface StoredObject {
	key: string;
	size: number;
	contentType: string;
}

export interface FakeS3 extends Fake {
	/** What the bucket holds right now, as the harness sees it (not through a signed URL). */
	objects(): StoredObject[];
}

const enc = (value: string): string =>
	encodeURIComponent(value).replace(
		/[!'()*]/g,
		(c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`,
	);

const sha256 = (value: string): string =>
	createHash("sha256").update(value).digest("hex");

const hmac = (key: Buffer | string, value: string): Buffer =>
	createHmac("sha256", key).update(value).digest();

const CORS: Record<string, string> = {
	"access-control-allow-origin": "*",
	"access-control-allow-methods": "PUT, GET, HEAD, DELETE",
	"access-control-expose-headers": "ETag",
};

/**
 * Stands in for the S3-compatible bucket the gateway signs URLs for (STORAGE_PROVIDER=s3, path
 * style, like MinIO). It accepts a request only when its SigV4 query-string signature is valid for the
 * request as it arrives: the signed headers (content type and length) must be the ones sent, and the
 * URL must not have expired. It is written independently of the Go signer, so a signing bug fails an
 * end-to-end upload here. It also answers the CORS preflight the browser sends before a cross-origin
 * PUT. `GET /__objects` lists what is stored.
 */
export function startFakeS3(options: {
	bucket: string;
	region: string;
	accessKeyId: string;
	secretAccessKey: string;
}): Promise<FakeS3> {
	const store = new Map<string, { data: Buffer; contentType: string }>();
	const objects = (): StoredObject[] =>
		[...store.entries()].map(([key, o]) => ({
			key,
			size: o.data.length,
			contentType: o.contentType,
		}));

	const verify = (
		method: string,
		url: URL,
		headers: Record<string, string | string[] | undefined>,
	): string | null => {
		const q = url.searchParams;
		if (q.get("X-Amz-Algorithm") !== "AWS4-HMAC-SHA256") return "AccessDenied";
		const credential = (q.get("X-Amz-Credential") ?? "").split("/");
		if (
			credential.length !== 5 ||
			credential[0] !== options.accessKeyId ||
			credential[2] !== options.region ||
			credential[3] !== "s3" ||
			credential[4] !== "aws4_request"
		) {
			return "InvalidAccessKeyId";
		}
		const stamp = q.get("X-Amz-Date") ?? "";
		const issued = Date.parse(
			stamp.replace(
				/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/,
				"$1-$2-$3T$4:$5:$6Z",
			),
		);
		const expires = Number(q.get("X-Amz-Expires"));
		if (
			Number.isNaN(issued) ||
			!(expires > 0 && expires <= 7 * 24 * 3600) ||
			Date.now() > issued + expires * 1000 ||
			Date.now() < issued - 60_000
		) {
			return "ExpiredToken";
		}

		const signedHeaders = q.get("X-Amz-SignedHeaders") ?? "";
		const names = signedHeaders.split(";");
		if (!names.includes("host")) return "AccessDenied";
		const canonicalHeaders = names
			.map((name) => {
				const value = headers[name];
				return `${name}:${(Array.isArray(value) ? value.join(",") : (value ?? "")).trim()}\n`;
			})
			.join("");
		const query = [...q.entries()]
			.filter(([k]) => k !== "X-Amz-Signature")
			.map(([k, v]) => [enc(k), enc(v)] as const)
			.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
			.map(([k, v]) => `${k}=${v}`)
			.join("&");
		const path = url.pathname
			.split("/")
			.map((segment) => enc(decodeURIComponent(segment)))
			.join("/");
		const canonical = [
			method,
			path,
			query,
			canonicalHeaders,
			signedHeaders,
			"UNSIGNED-PAYLOAD",
		].join("\n");
		const scope = credential.slice(1).join("/");
		const toSign = ["AWS4-HMAC-SHA256", stamp, scope, sha256(canonical)].join(
			"\n",
		);
		let key = hmac(`AWS4${options.secretAccessKey}`, credential[1] as string);
		for (const part of [credential[2], credential[3], credential[4]]) {
			key = hmac(key, part as string);
		}
		const want = Buffer.from(hmac(key, toSign).toString("hex"));
		const got = Buffer.from(q.get("X-Amz-Signature") ?? "");
		return want.length === got.length && timingSafeEqual(want, got)
			? null
			: "SignatureDoesNotMatch";
	};

	return startServer((req, res, body) => {
		const url = new URL(req.url ?? "/", "http://fake");
		const method = req.method ?? "GET";
		if (method === "OPTIONS") {
			// A bucket's CORS rule lists the allowed headers; the harness browsers also send a
			// per-context X-Forwarded-For on every request, so the preflight's list is echoed back.
			res.writeHead(204, {
				...CORS,
				"access-control-allow-headers": String(
					req.headers["access-control-request-headers"] ?? "content-type",
				),
			});
			res.end();
			return;
		}
		if (method === "GET" && url.pathname === "/__objects") {
			return json(res, 200, objects());
		}

		const [, bucket, ...rest] = url.pathname.split("/");
		const key = rest.map(decodeURIComponent).join("/");
		const fail = (status: number, code: string) => {
			res.writeHead(status, { ...CORS, "content-type": "application/xml" });
			res.end(`<Error><Code>${code}</Code></Error>`);
		};
		if (bucket !== options.bucket || !key) return fail(404, "NoSuchBucket");
		const denied = verify(method, url, req.headers);
		if (denied) return fail(403, denied);

		switch (method) {
			case "PUT": {
				const contentType = String(req.headers["content-type"] ?? "");
				store.set(key, { data: body, contentType });
				res.writeHead(200, {
					...CORS,
					etag: `"${sha256(body.toString("binary"))}"`,
				});
				res.end();
				return;
			}
			case "GET":
			case "HEAD": {
				const object = store.get(key);
				if (!object) return fail(404, "NoSuchKey");
				const headers: Record<string, string | number> = {
					...CORS,
					"content-type": object.contentType,
					"content-length": object.data.length,
				};
				const disposition = url.searchParams.get(
					"response-content-disposition",
				);
				if (disposition) headers["content-disposition"] = disposition;
				res.writeHead(200, headers);
				res.end(method === "GET" ? object.data : undefined);
				return;
			}
			case "DELETE":
				store.delete(key);
				res.writeHead(204, CORS);
				res.end();
				return;
			default:
				return fail(405, "MethodNotAllowed");
		}
	}).then((fake) => ({ ...fake, objects }));
}

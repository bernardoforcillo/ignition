import {
	createServer,
	type IncomingMessage,
	type Server,
	type ServerResponse,
} from "node:http";
import type { AddressInfo } from "node:net";

export interface Fake {
	url: string;
	close(): Promise<void>;
}

export type Handler = (
	req: IncomingMessage,
	res: ServerResponse,
	body: Buffer,
) => void | Promise<void>;

/** Starts a loopback HTTP server on a free port; `handler` gets the fully read body. */
export async function startServer(handler: Handler): Promise<Fake> {
	const server: Server = createServer((req, res) => {
		const chunks: Buffer[] = [];
		req.on("data", (chunk: Buffer) => chunks.push(chunk));
		req.on("end", () => {
			Promise.resolve(handler(req, res, Buffer.concat(chunks))).catch(
				(error: unknown) => {
					res.statusCode = 500;
					res.end(String(error));
				},
			);
		});
	});
	await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
	const { port } = server.address() as AddressInfo;
	return {
		url: `http://127.0.0.1:${port}`,
		close: () =>
			new Promise<void>((resolve) => {
				server.closeAllConnections();
				server.close(() => resolve());
			}),
	};
}

export function json(
	res: ServerResponse,
	status: number,
	value: unknown,
): void {
	res.writeHead(status, { "content-type": "application/json" });
	res.end(JSON.stringify(value));
}

export function html(res: ServerResponse, title: string): void {
	res.writeHead(200, { "content-type": "text/html" });
	res.end(`<!doctype html><title>${title}</title><h1>${title}</h1>`);
}

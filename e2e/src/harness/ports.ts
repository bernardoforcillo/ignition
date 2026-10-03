import { createServer } from "node:net";

/** Asks the OS for a free loopback port. Two calls never return the same port while both are held. */
export async function freePort(): Promise<number> {
	const server = createServer();
	await new Promise<void>((resolve, reject) => {
		server.once("error", reject);
		server.listen(0, "127.0.0.1", resolve);
	});
	const address = server.address();
	await new Promise<void>((resolve) => server.close(() => resolve()));
	if (typeof address === "string" || !address) throw new Error("no port");
	return address.port;
}

import { type ChildProcess, spawn } from "node:child_process";
import { mkdirSync, openSync } from "node:fs";
import path from "node:path";

export interface Managed {
	child: ChildProcess;
	stop(): Promise<void>;
}

/** Runs a command to completion, failing with its output when it exits non-zero. */
export function run(
	command: string,
	args: string[],
	options: { cwd: string; env?: NodeJS.ProcessEnv; log: string },
): Promise<void> {
	return new Promise((resolve, reject) => {
		const out = openLog(options.log);
		const child = spawn(command, args, {
			cwd: options.cwd,
			env: options.env ?? process.env,
			stdio: ["ignore", out, out],
		});
		child.once("error", reject);
		child.once("exit", (code) =>
			code === 0
				? resolve()
				: reject(
						new Error(
							`${command} ${args.join(" ")} exited with ${code}; see ${options.log}`,
						),
					),
		);
	});
}

/** Starts a long-running process, logging to a file; `stop` ends it and waits. */
export function start(
	command: string,
	args: string[],
	options: { cwd: string; env: NodeJS.ProcessEnv; log: string },
): Managed {
	const out = openLog(options.log);
	const child = spawn(command, args, {
		cwd: options.cwd,
		env: options.env,
		stdio: ["ignore", out, out],
		detached: true,
	});
	return {
		child,
		stop: () =>
			new Promise<void>((resolve) => {
				if (child.exitCode !== null || child.signalCode !== null)
					return resolve();
				child.once("exit", () => resolve());
				// The whole group: `pnpm exec` and `vite` leave grandchildren behind otherwise.
				try {
					process.kill(-(child.pid as number), "SIGTERM");
				} catch {
					child.kill("SIGTERM");
				}
				setTimeout(() => {
					try {
						process.kill(-(child.pid as number), "SIGKILL");
					} catch {
						// already gone
					}
				}, 5000).unref();
			}),
	};
}

function openLog(file: string): number {
	mkdirSync(path.dirname(file), { recursive: true });
	return openSync(file, "w");
}

/** Polls `url` until it answers 2xx, or throws after `timeoutMs` (or when `alive` turns false). */
export async function waitForHttp(
	url: string,
	timeoutMs: number,
	alive: () => boolean,
): Promise<void> {
	const deadline = Date.now() + timeoutMs;
	let last = "no response";
	while (Date.now() < deadline) {
		if (!alive()) throw new Error(`process exited while waiting for ${url}`);
		try {
			const res = await fetch(url);
			if (res.ok) return;
			last = `status ${res.status}`;
		} catch (error) {
			last = String(error);
		}
		await new Promise((resolve) => setTimeout(resolve, 150));
	}
	throw new Error(`${url} not ready after ${timeoutMs}ms (${last})`);
}

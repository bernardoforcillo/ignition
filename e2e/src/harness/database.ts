import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";

export const DEFAULT_DATABASE_URL =
	"postgres://postgres@/ignition_e2e?host=/var/tmp/igpg&port=55432&sslmode=disable";

const PSQL_FALLBACK = "/usr/lib/postgresql/16/bin/psql";

/** Drops and recreates the e2e database so every run starts empty and migrations run from scratch. */
export function resetDatabase(databaseUrl: string): void {
	// Not `new URL`: a socket-dir DSN has an empty host, which WHATWG URL rejects.
	const parts = /^(postgres(?:ql)?:\/\/[^/?]*)\/([^?]*)(\?.*)?$/.exec(
		databaseUrl,
	);
	if (!parts)
		throw new Error(
			"E2E_DATABASE_URL is not a postgres:// URL with a database name",
		);
	const [, origin, rawName = "", query = ""] = parts;
	const name = decodeURIComponent(rawName);
	// This DROPs a database: refuse anything that does not announce itself as a throwaway.
	if (!/^[a-z0-9_]*e2e[a-z0-9_]*$/.test(name)) {
		throw new Error(
			`E2E_DATABASE_URL must name a database containing "e2e" (got "${name}"): it is dropped on every run`,
		);
	}
	const adminUrl = `${origin}/postgres${query}`;
	const psql =
		process.env.E2E_PSQL ??
		(existsSync(PSQL_FALLBACK) ? PSQL_FALLBACK : "psql");
	const run = (sql: string) =>
		execFileSync(
			psql,
			["-X", "-q", "-v", "ON_ERROR_STOP=1", adminUrl, "-c", sql],
			{ stdio: ["ignore", "pipe", "pipe"] },
		);
	run(`DROP DATABASE IF EXISTS ${name} WITH (FORCE)`);
	run(`CREATE DATABASE ${name}`);
}

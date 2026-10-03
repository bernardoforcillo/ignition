import { fileURLToPath, URL } from "node:url";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { loadEnv } from "vite";
import { defineConfig } from "vitest/config";

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, process.cwd(), "");
	// Connect RPCs are POSTs to `/<package>.<Service>/<Method>`. The dev and preview servers
	// forward them to the gateway, so the browser keeps talking to its own origin (no CORS).
	const target = env.VITE_API_PROXY_TARGET || "http://localhost:8080";
	const proxy = {
		"/saas.v1.": { target, changeOrigin: true },
		"/gateway.v1.": { target, changeOrigin: true },
	};

	return {
		plugins: [react(), tailwindcss()],
		resolve: {
			alias: {
				// Kept in sync with the `~` entry in tsconfig.json's compilerOptions.paths.
				"~": fileURLToPath(new URL("./src", import.meta.url)),
			},
		},
		server: { proxy },
		preview: { proxy },
		test: {
			environment: "node",
			include: ["src/**/*.test.ts"],
		},
	};
});

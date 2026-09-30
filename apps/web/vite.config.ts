import { fileURLToPath, URL } from "node:url";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			// Kept in sync with the `~` entry in tsconfig.json's compilerOptions.paths.
			"~": fileURLToPath(new URL("./src", import.meta.url)),
		},
	},
});

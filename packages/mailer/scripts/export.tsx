/**
 * Renders every template to static HTML + plain text with Go template placeholders
 * (`{{.Link}}`) in place of the props, and writes them where `go-packages/mailer` embeds them.
 *
 *   pnpm gen:email    write the files (generated, never committed: see go-packages/mailer)
 */
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { render } from "@react-email/render";
import { createElement } from "react";

import { plainTextOptions } from "../src/plain-text";

// Must be set before the templates are imported: `src/emails/assets.ts` reads it at load time.
process.env.EMAIL_ASSET_BASE_URL = "{{.AssetBaseUrl}}";
const { templates } = await import("../src/templates");

const outDir = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../../../go-packages/mailer/templates",
);

const pascal = (value: string) => value[0].toUpperCase() + value.slice(1);

const files = new Map<string, string>();
const manifest: Record<string, { subject: string; variables: string[] }> = {};

for (const [name, definition] of Object.entries(templates)) {
	const placeholders = Object.fromEntries(
		definition.variables.map((key) => [key, `{{.${pascal(key)}}}`]),
	);
	// biome-ignore lint/suspicious/noExplicitAny: the registry is typed per template, the export is generic.
	const element = createElement(definition.component as any, placeholders);
	files.set(`${name}.html`, await render(element));
	files.set(`${name}.txt`, await render(element, plainTextOptions));
	manifest[name] = {
		// biome-ignore lint/suspicious/noExplicitAny: same as above.
		subject: (definition.subject as any)(placeholders),
		// `assetBaseUrl` is not a template prop: every body references it through `assets.ts`.
		variables: [...definition.variables, "assetBaseUrl"].map(pascal),
	};
}
files.set("manifest.json", `${JSON.stringify(manifest, null, "\t")}\n`);

for (const [file, contents] of files) {
	const target = path.join(outDir, file);
	// Read and catch instead of checking first, so there is no gap between check and use.
	let current: string | null = null;
	try {
		current = readFileSync(target, "utf8");
	} catch {
		// Not exported yet.
	}
	if (current === contents) continue;
	mkdirSync(outDir, { recursive: true });
	writeFileSync(target, contents);
	console.log(`wrote ${path.relative(process.cwd(), target)}`);
}

/**
 * Renders every template to static HTML + plain text with Go template placeholders
 * (`{{.Link}}`) in place of the props, and writes them where `go-packages/mailer` embeds them.
 *
 *   pnpm --filter @ignition/mailer export          write the files
 *   pnpm --filter @ignition/mailer check:export    fail if the committed files are stale
 */
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { render } from "@react-email/render";
import { createElement } from "react";

import { plainTextOptions } from "../src/plain-text";
import { templates } from "../src/templates";

const outDir = path.resolve(
	path.dirname(fileURLToPath(import.meta.url)),
	"../../../go-packages/mailer/templates",
);
const check = process.argv.includes("--check");

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
		variables: definition.variables.map(pascal),
	};
}
files.set("manifest.json", `${JSON.stringify(manifest, null, "\t")}\n`);

let stale = 0;
for (const [file, contents] of files) {
	const target = path.join(outDir, file);
	const current = existsSync(target) ? readFileSync(target, "utf8") : null;
	if (current === contents) continue;
	if (check) {
		console.error(`stale: ${path.relative(process.cwd(), target)}`);
		stale++;
		continue;
	}
	mkdirSync(outDir, { recursive: true });
	writeFileSync(target, contents);
	console.log(`wrote ${path.relative(process.cwd(), target)}`);
}
if (check && stale > 0) {
	console.error(
		"Run `pnpm --filter @ignition/mailer export` and commit the result.",
	);
	process.exit(1);
}

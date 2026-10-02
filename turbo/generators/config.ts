import type { PlopTypes } from "@turbo/gen";

const KEBAB_CASE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const kebab = (value: string) =>
	KEBAB_CASE.test(value) || "Use lowercase kebab-case";
const port = (value: string) =>
	/^\d{2,5}$/.test(value) || "Enter a port number";

const WEB = "apps/web/src";
const COMPONENTS = "packages/components/src";
const K8S = "infrastructure/kubernetes";
const ENVIRONMENTS = [
	{ env: "development", replicas: 1 },
	{ env: "production", replicas: 2 },
] as const;

/** Append `line` to a file unless it is already there (idempotent). */
const appendLine = (line: string) => (contents: string) =>
	contents.includes(line)
		? contents
		: `${contents.replace(/\n*$/, "\n")}${line}\n`;

export default function generator(plop: PlopTypes.NodePlopAPI): void {
	plop.setGenerator("feature", {
		description:
			"Scaffold a web feature: feature folder, api module, route, and wire it in",
		prompts: [
			{
				type: "input",
				name: "name",
				message: "Feature name (kebab-case, e.g. billing):",
				validate: kebab,
			},
		],
		actions: [
			{
				type: "add",
				path: `${WEB}/features/{{name}}/index.ts`,
				templateFile: "templates/feature/index.ts.hbs",
			},
			{
				type: "add",
				path: `${WEB}/features/{{name}}/{{name}}-panel.tsx`,
				templateFile: "templates/feature/panel.tsx.hbs",
			},
			{
				type: "add",
				path: `${WEB}/lib/api/{{name}}.ts`,
				templateFile: "templates/feature/api.ts.hbs",
			},
			{
				type: "modify",
				path: `${WEB}/lib/api/index.ts`,
				transform: (contents, data) =>
					appendLine(`export * from "./${data.name}";`)(contents),
			},
			{
				type: "add",
				path: `${WEB}/routes/{{name}}/index.ts`,
				templateFile: "templates/feature/route.ts.hbs",
			},
			{
				type: "modify",
				path: `${WEB}/routes/index.ts`,
				transform: (contents, data) => {
					const camel = plop.getHelper("camelCase") as (s: string) => string;
					const route = `${camel(data.name)}Route`;
					return contents
						.replace(
							/(import \{ rootRoute \} from "~\/routes\/root";)/,
							`import { ${route} } from "~/routes/${data.name}";\n$1`,
						)
						.replace(
							/addChildren\(\[([^\]]*)\]\)/,
							(_all, list: string) =>
								`addChildren([${list.trim().replace(/,$/, "")}, ${route}])`,
						);
				},
			},
			{
				type: "modify",
				path: `${WEB}/routes/root/root-layout.tsx`,
				transform: (contents, data) => {
					const title = (plop.getHelper("titleCase") as (s: string) => string)(
						data.name,
					);
					return contents.replace(
						/(\s*)<\/nav>/,
						`$1\t<Link to="/${data.name}" activeProps={{ className: "text-brand-600" }}>\n\t\t\t\t\t\t${title}\n\t\t\t\t\t</Link>$1</nav>`,
					);
				},
			},
		],
	});

	plop.setGenerator("component", {
		description: "Scaffold a shared atom or molecule in @ignition/components",
		prompts: [
			{
				type: "list",
				name: "tier",
				message: "Tier:",
				choices: ["atoms", "molecules"],
			},
			{
				type: "input",
				name: "name",
				message: "Component name (kebab-case, e.g. badge):",
				validate: kebab,
			},
		],
		actions: [
			{
				type: "add",
				path: `${COMPONENTS}/{{tier}}/{{name}}/{{name}}.tsx`,
				templateFile: "templates/component/component.tsx.hbs",
			},
			{
				type: "add",
				path: `${COMPONENTS}/{{tier}}/{{name}}/index.ts`,
				templateFile: "templates/component/index.ts.hbs",
			},
			{
				type: "modify",
				path: `${COMPONENTS}/{{tier}}/index.ts`,
				transform: (contents, data) =>
					appendLine(`export * from "./${data.name}";`)(contents),
			},
		],
	});

	plop.setGenerator("deployment", {
		description: "Scaffold the same Kubernetes deployment in every environment",
		prompts: [
			{
				type: "input",
				name: "name",
				message: "Deployment name (kebab-case, e.g. billing):",
				validate: kebab,
			},
			{
				type: "input",
				name: "port",
				message: "Container port:",
				default: "8080",
				validate: port,
			},
		],
		actions: deploymentActions(),
	});

	plop.setGenerator("service", {
		description:
			"Scaffold a Go service in apps/<name>: main, config, http transport, Dockerfile, go.work entry and manifests",
		prompts: [
			{
				type: "input",
				name: "name",
				message: "Service name (kebab-case, e.g. billing-api):",
				validate: kebab,
			},
			{
				type: "input",
				name: "port",
				message: "Listen port:",
				default: "8080",
				validate: port,
			},
			{
				type: "confirm",
				name: "manifests",
				message: "Also scaffold the Kubernetes manifests in every environment?",
				default: true,
			},
		],
		actions: (data) => {
			const dir = "apps/{{name}}";
			const files: [string, string][] = [
				["go.mod", "go.mod.hbs"],
				["main.go", "main.go.hbs"],
				["Dockerfile", "Dockerfile.hbs"],
				[".air.toml", "air.toml.hbs"],
				[".gitignore", "gitignore.hbs"],
				["README.md", "README.md.hbs"],
				["internal/config/config.go", "config.go.hbs"],
				["internal/config/config_test.go", "config_test.go.hbs"],
				["internal/adapter/httpapi/server.go", "server.go.hbs"],
				["internal/adapter/httpapi/health.go", "health.go.hbs"],
				["internal/adapter/httpapi/health_test.go", "health_test.go.hbs"],
			];
			const actions: PlopTypes.ActionType[] = files.map(
				([target, template]): PlopTypes.ActionType => ({
					type: "add",
					path: `${dir}/${target}`,
					templateFile: `templates/service/${template}`,
				}),
			);
			actions.push({
				type: "modify",
				path: "go.work",
				// Register the module after the last apps/ entry; idempotent.
				transform: (contents, answers) => {
					const line = `\t./apps/${answers.name}\n`;
					if (contents.includes(line)) return contents;
					return contents.replace(
						/(\t\.\/apps\/[^\n]+\n)(?!\t\.\/apps\/)/,
						`$1${line}`,
					);
				},
			});
			if (data?.manifests) actions.push(...deploymentActions());
			return actions;
		},
	});
}

/** The same deployment, service and PDB in every environment, registered in each kustomization. */
function deploymentActions(): PlopTypes.ActionType[] {
	return ENVIRONMENTS.flatMap(({ env, replicas }) => {
		const dir = `${K8S}/${env}/ignition-${env}`;
		const extra = { env, replicas };
		return [
			...["deployment", "service", "pdb", "kustomization"].map(
				(file): PlopTypes.ActionType => ({
					type: "add",
					path: `${dir}/{{name}}/${file}.yaml`,
					templateFile: `templates/deployment/${file}.yaml.hbs`,
					data: extra,
				}),
			),
			{
				type: "modify",
				path: `${dir}/kustomization.yaml`,
				transform: (contents, data) => appendLine(`  - ${data.name}`)(contents),
			} satisfies PlopTypes.ActionType,
		];
	});
}

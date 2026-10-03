import { type BrowserContext, test as base } from "@playwright/test";

import { newContext, signedInWithWorkspace, uniqueName } from "./users";

type WorkspaceUser = Awaited<ReturnType<typeof signedInWithWorkspace>>;

/**
 * `isolated()` opens a browser context with its own storage and client IP, closed after the test;
 * call it again for a second user. `workspaceUser()` is that plus a verified, signed-in account that
 * has created a workspace.
 */
export const test = base.extend<{
	isolated: () => Promise<BrowserContext>;
	workspaceUser: (workspaceName?: string) => Promise<WorkspaceUser>;
}>({
	isolated: async ({ browser, baseURL }, use) => {
		const opened: BrowserContext[] = [];
		await use(async () => {
			const context = await newContext(browser, baseURL as string);
			opened.push(context);
			return context;
		});
		await Promise.all(opened.map((context) => context.close()));
	},
	workspaceUser: async ({ isolated }, use) => {
		await use(async (name) =>
			signedInWithWorkspace(await isolated(), name && uniqueName(name)),
		);
	},
});

export { expect } from "@playwright/test";

import { readFile } from "node:fs/promises";

import type { Page } from "@playwright/test";

import { sendSubscriptionEvent } from "../src/support/stripe";
import { expect, test } from "../src/support/test";
import { currentWorkspaceId, PASSWORD, signIn } from "../src/support/users";

async function openDeleteDialog(page: Page) {
	await page.goto("/app/settings");
	await page.getByRole("button", { name: "Delete account" }).click();
	return page.getByRole("dialog");
}

async function confirmDelete(page: Page, password: string) {
	const dialog = page.getByRole("dialog");
	await dialog.getByLabel("Confirm your password").fill(password);
	await dialog.getByRole("button", { name: "Delete my account" }).click();
}

test.describe("GDPR", () => {
	test("export downloads the account's data without secrets", async ({
		workspaceUser,
	}) => {
		const { page, account, workspaceName } = await workspaceUser("Export Co");
		await page.getByRole("link", { name: "Settings" }).click();
		await expect(page.getByText(account.email, { exact: true })).toBeVisible();

		const download = page.waitForEvent("download");
		await page.getByRole("button", { name: "Export my data" }).click();
		const file = await download;
		expect(file.suggestedFilename()).toMatch(/\.json$/);

		const raw = await readFile(await file.path(), "utf8");
		const data = JSON.parse(raw) as unknown; // valid JSON
		expect(data).toBeTruthy();
		expect(raw).toContain(account.email);
		expect(raw).toContain(workspaceName);
		// Nothing a leak could be replayed with: no password, hash or token material.
		expect(raw).not.toContain(PASSWORD);
		expect(raw).not.toMatch(/argon|bcrypt|scrypt|\$2[aby]\$|pbkdf/i);
		expect(raw).not.toMatch(/password|token|secret|hash/i);
	});

	test("deleting the account needs the right password, then erases it", async ({
		workspaceUser,
		isolated,
	}) => {
		const { page, account } = await workspaceUser("Erase Co");
		await openDeleteDialog(page);

		await confirmDelete(page, "not-my-Password-1!");
		await expect(page.getByRole("dialog").getByRole("alert")).toBeVisible();
		await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
			/incorrect password/i,
		);
		await expect(page).toHaveURL(/\/app\/settings$/);

		// The failed attempt left the session intact.
		await page.getByRole("button", { name: "Cancel" }).click();
		await page.reload();
		await expect(
			page.getByRole("heading", { name: "Settings", level: 1 }),
		).toBeVisible();

		await page.getByRole("button", { name: "Delete account" }).click();
		await confirmDelete(page, account.password);
		await expect(page).toHaveURL(/\/login$/);

		// Gone: the same credentials no longer sign in, from any browser.
		const other = await (await isolated()).newPage();
		await signIn(other, account);
		await expect(other.getByRole("alert")).toHaveText(
			"Invalid email or password",
		);
		await expect(other).toHaveURL(/\/login/);
	});

	test("deletion is blocked while Stripe still bills the workspace", async ({
		workspaceUser,
	}) => {
		const { page, account } = await workspaceUser("Billed Co");
		const workspaceId = await currentWorkspaceId(page);
		expect(
			(
				await sendSubscriptionEvent({
					type: "customer.subscription.created",
					workspaceId,
				})
			).status,
		).toBe(200);

		await openDeleteDialog(page);
		await confirmDelete(page, account.password);
		const alert = page.getByRole("dialog").getByRole("alert");
		await expect(alert).toBeVisible();
		await expect(alert).toContainText(/subscription|billing|cancel/i);
		await expect(page).toHaveURL(/\/app\/settings$/);

		// Cancel at the provider, and the same request now succeeds.
		expect(
			(
				await sendSubscriptionEvent({
					type: "customer.subscription.deleted",
					workspaceId,
					status: "canceled",
				})
			).status,
		).toBe(200);
		await page
			.getByRole("dialog")
			.getByRole("button", { name: "Delete my account" })
			.click();
		await expect(page).toHaveURL(/\/login$/);
	});
});

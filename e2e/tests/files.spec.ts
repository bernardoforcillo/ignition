import { readFile } from "node:fs/promises";

import type { Page } from "@playwright/test";
import {
	accessToken,
	reserveStorage,
	storedObjects,
} from "../src/support/files";
import { waitForLink } from "../src/support/mail";
import { expect, test } from "../src/support/test";
import {
	createVerifiedAccount,
	currentWorkspaceId,
	PASSWORD,
	signIn,
	uniqueEmail,
} from "../src/support/users";

async function openFiles(page: Page) {
	await page.getByRole("link", { name: "Files" }).click();
	await expect(
		page.getByRole("heading", { name: "Files", level: 1 }),
	).toBeVisible();
}

const upload = (page: Page, name: string, mimeType: string, body: string) =>
	page
		.getByLabel("Upload files")
		.setInputFiles({ name, mimeType, buffer: Buffer.from(body) });

test.describe("files", () => {
	test("upload straight to storage, list, download and delete", async ({
		workspaceUser,
	}) => {
		const { page } = await workspaceUser("Files Co");
		const workspaceId = await currentWorkspaceId(page);
		await openFiles(page);

		await expect(page.getByText("No files yet")).toBeVisible();
		await expect(page.getByText("0 B of 100 MB used")).toBeVisible();
		await expect(
			page.getByRole("progressbar", { name: "Storage used" }),
		).toBeVisible();

		await upload(page, "notes.txt", "text/plain", "hello e2e");
		await expect(page.getByText("Uploaded", { exact: true })).toBeVisible();
		// Only the file list has a Download button, so this is the stored file, not the upload row.
		const row = page.getByRole("listitem").filter({
			has: page.getByRole("button", { name: "Download notes.txt" }),
		});
		await expect(row).toContainText("9 B");
		await expect(page.getByText("9 B of 100 MB used")).toBeVisible();

		// The bytes went to the bucket under a generated key, with the type that was signed.
		const stored = await storedObjects(workspaceId);
		expect(stored).toHaveLength(1);
		expect(stored[0]).toMatchObject({ size: 9, contentType: "text/plain" });
		expect(stored[0]?.key).toMatch(
			new RegExp(`^workspaces/${workspaceId}/[0-9a-f-]{36}$`),
		);

		// Download: a short-lived signed URL whose response is an attachment under the original name.
		const downloading = page.waitForEvent("download");
		await page.getByRole("button", { name: "Download notes.txt" }).click();
		const download = await downloading;
		expect(download.suggestedFilename()).toBe("notes.txt");
		expect(await readFile((await download.path()) as string, "utf8")).toBe(
			"hello e2e",
		);

		// Delete asks first, then removes the record and the object.
		await page.getByRole("button", { name: "Delete notes.txt" }).click();
		await page.getByRole("button", { name: "Cancel", exact: true }).click();
		await expect(row).toBeVisible();
		await page.getByRole("button", { name: "Delete notes.txt" }).click();
		await page
			.getByRole("button", { name: "Confirm delete notes.txt" })
			.click();
		await expect(page.getByText("No files yet")).toBeVisible();
		await expect(page.getByText("0 B of 100 MB used")).toBeVisible();
		await expect.poll(() => storedObjects(workspaceId)).toEqual([]);
	});

	test("a workspace never sees another workspace's files", async ({
		workspaceUser,
	}) => {
		const ada = await workspaceUser("Ada Files");
		const bob = await workspaceUser("Bob Files");
		await openFiles(ada.page);
		await upload(ada.page, "private.txt", "text/plain", "ada only");
		await expect(
			ada.page.getByRole("button", { name: "Download private.txt" }),
		).toBeVisible();

		await openFiles(bob.page);
		await expect(bob.page.getByText("No files yet")).toBeVisible();
		await expect(bob.page.getByText("private.txt")).toHaveCount(0);
	});

	test("refuses a type the product does not allow, and stores nothing", async ({
		workspaceUser,
	}) => {
		const { page } = await workspaceUser("Types Co");
		const workspaceId = await currentWorkspaceId(page);
		await openFiles(page);

		await upload(page, "page.html", "text/html", "<script>alert(1)</script>");
		await expect(page.getByRole("alert")).toContainText(
			"this file type is not allowed",
		);
		await expect(page.getByText("No files yet")).toBeVisible();
		expect(await storedObjects(workspaceId)).toEqual([]);
	});

	test("a full workspace is told it has run out of storage", async ({
		workspaceUser,
	}) => {
		const { page, account } = await workspaceUser("Quota Co");
		const workspaceId = await currentWorkspaceId(page);
		const token = await accessToken(page.request, account);
		// Take all but 10 bytes of the free plan's 100 MiB.
		await reserveStorage(
			page.request,
			token,
			workspaceId,
			100 * 1024 * 1024 - 10,
		);

		await openFiles(page);
		await expect(page.getByText("100 MB of 100 MB used")).toBeVisible();
		await expect(page.getByText("Storage is full.")).toBeVisible();

		await upload(page, "too-big.txt", "text/plain", "x".repeat(50));
		await expect(page.getByRole("alert")).toContainText("run out of storage");
		expect(await storedObjects(workspaceId)).toEqual([]);
	});

	test("a member without file permissions gets an explanation, not a broken page", async ({
		workspaceUser,
		isolated,
	}) => {
		const owner = await workspaceUser("Roles Co");
		await owner.page.getByRole("link", { name: "Members" }).click();
		const memberEmail = uniqueEmail("member");
		await owner.page.getByLabel("Email address").fill(memberEmail);
		await owner.page.getByRole("button", { name: "Send invitation" }).click();
		await expect(owner.page.getByText("Invitation sent.")).toBeVisible();

		const guestContext = await isolated();
		await createVerifiedAccount(guestContext.request, memberEmail);
		const guest = await guestContext.newPage();
		await signIn(guest, { email: memberEmail, password: PASSWORD });
		await expect(guest).toHaveURL(/\/onboarding$/);
		await guest.goto(await waitForLink(memberEmail, "/invite/accept"));
		await expect(guest).toHaveURL(/\/app$/);

		await openFiles(guest);
		await expect(guest.getByRole("alert")).toContainText("owner or admin");
		await expect(guest.getByLabel("Upload files")).toHaveCount(0);
	});
});

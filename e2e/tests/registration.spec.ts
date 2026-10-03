import type { Page } from "@playwright/test";

import { allMail, waitForLink } from "../src/support/mail";
import { expect, test } from "../src/support/test";
import {
	createVerifiedAccount,
	createWorkspace,
	PASSWORD,
	signIn,
	uniqueEmail,
	uniqueName,
} from "../src/support/users";

async function signUp(page: Page, email: string) {
	await page.goto("/signup");
	await page.getByLabel("Email").fill(email);
	await page.getByLabel("Password", { exact: true }).fill(PASSWORD);
	await page.getByRole("button", { name: "Create account" }).click();
}

test.describe("registration", () => {
	test("sign up, verify by email, sign in, create a workspace", async ({
		isolated,
	}) => {
		const page = await (await isolated()).newPage();
		const email = uniqueEmail("reg");

		await signUp(page, email);
		await expect(
			page.getByRole("heading", { name: "Check your email" }),
		).toBeVisible();
		await expect(page.getByText(email)).toBeVisible();

		const link = await waitForLink(email, "/verify-email");
		await page.goto(link);
		await expect(
			page.getByRole("heading", { name: "Email verified" }),
		).toBeVisible();

		await page.getByRole("link", { name: "Continue to sign in" }).click();
		await signIn(page, { email, password: PASSWORD }, "/login");
		await createWorkspace(page, uniqueName("Registration Co"));

		await expect(page.getByRole("link", { name: "Members" })).toBeVisible();
		await expect(page.getByLabel("Main")).toBeVisible();
	});

	test("an unverified user cannot sign in, with a safe message", async ({
		isolated,
	}) => {
		const page = await (await isolated()).newPage();
		const email = uniqueEmail("unverified");
		await signUp(page, email);
		await expect(
			page.getByRole("heading", { name: "Check your email" }),
		).toBeVisible();

		await signIn(page, { email, password: PASSWORD });
		const alert = page.getByRole("alert");
		await expect(alert).toBeVisible();
		// Same wording as a wrong password: nothing about the account's state leaks.
		await expect(alert).toHaveText("Invalid email or password");
		await expect(page).toHaveURL(/\/login/);
	});

	test("signing up twice looks the same as signing up once (no enumeration)", async ({
		isolated,
	}) => {
		const context = await isolated();
		const fresh = await context.newPage();
		const repeat = await context.newPage();
		const newEmail = uniqueEmail("new");
		const takenEmail = uniqueEmail("dupe");
		await createVerifiedAccount(context.request, takenEmail);

		await signUp(fresh, newEmail);
		await signUp(repeat, takenEmail);

		// The visible outcome is identical, address aside.
		const outcome = async (page: Page, email: string) => {
			await expect(
				page.getByRole("heading", { name: "Check your email" }),
			).toBeVisible();
			return (await page.locator("main").innerText()).replaceAll(
				email,
				"<email>",
			);
		};
		expect(await outcome(repeat, takenEmail)).toBe(
			await outcome(fresh, newEmail),
		);

		// The existing owner is told by email instead, and no second verification link is issued.
		await expect.poll(async () => (await allMail(takenEmail)).length).toBe(2);
		const mails = await allMail(takenEmail);
		expect(mails.filter((m) => m.text.includes("/verify-email?"))).toHaveLength(
			1,
		);
		expect(mails.at(-1)?.text).toContain("/login");
	});
});

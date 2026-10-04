import type { Page } from "@playwright/test";

import { allMail, tokenOf, waitForLink } from "../src/support/mail";
import { expect, test } from "../src/support/test";
import {
	createVerifiedAccount,
	PASSWORD,
	signIn,
	uniqueEmail,
} from "../src/support/users";

const NEW_PASSWORD = "Another-Strong-7-Passphrase!";

test("password reset: neutral request, one-shot link, weak password refused", async ({
	isolated,
}) => {
	const context = await isolated();
	const email = uniqueEmail("reset");
	const unknown = uniqueEmail("nobody");
	await createVerifiedAccount(context.request, email);

	// Unknown and known addresses get the same screen.
	const request = async (target: Page, address: string) => {
		await target.goto("/forgot-password");
		await target.getByLabel("Email").fill(address);
		await target.getByRole("button", { name: "Send reset link" }).click();
		await expect(target.getByText("If an account exists for")).toBeVisible();
		return (await target.locator("main").innerText()).replaceAll(
			address,
			"<email>",
		);
	};
	// (One page after the other: a background tab's animation frames are throttled, which makes
	// Playwright's "stable" checks crawl.)
	const unknownScreen = await request(await context.newPage(), unknown);
	const page = await context.newPage();
	const knownScreen = await request(page, email);
	expect(knownScreen).toBe(unknownScreen);

	const link = await waitForLink(email, "/reset-password");
	// The unknown address was asked first; once the known one's mail is out, nothing went to it.
	expect(await allMail(unknown)).toEqual([]);

	await page.goto(link);
	const fill = async (password: string, confirm = password) => {
		await page.getByLabel("New password", { exact: true }).fill(password);
		await page.getByLabel("Confirm new password").fill(confirm);
		await page.getByRole("button", { name: "Reset password" }).click();
	};

	// A weak password is stopped by the form (which states the policy) and, if the form is bypassed,
	// by the server. Neither spends the token.
	await fill("short");
	await expect(page.getByText(/at least 12 characters/i).last()).toBeVisible();
	const weak = await context.request.post(
		"/saas.v1.AuthService/ResetPassword",
		{
			data: { token: tokenOf(link), newPassword: "alllowercasepassword" },
		},
	);
	expect(weak.status()).toBe(400);
	await expect(
		page.getByRole("heading", { name: "Reset password" }),
	).toBeVisible();

	await fill(NEW_PASSWORD);
	await expect(
		page.getByRole("heading", { name: "Password updated" }),
	).toBeVisible();

	// Old password is dead, the new one works.
	await signIn(page, { email, password: PASSWORD });
	await expect(page.getByRole("alert")).toHaveText("Invalid email or password");
	await signIn(page, { email, password: NEW_PASSWORD });
	await expect(page).toHaveURL(/\/onboarding$/);

	// The link cannot be used a second time.
	await page.getByRole("button", { name: "Sign out" }).click();
	await page.goto(link);
	await fill("Yet-Another-Strong-3-Phrase!");
	await expect(page.getByRole("alert")).toContainText("invalid or has expired");
	await signIn(page, { email, password: NEW_PASSWORD });
	await expect(page).toHaveURL(/\/onboarding$/);
});

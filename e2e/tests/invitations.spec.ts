import { waitForLink } from "../src/support/mail";
import { expect, test } from "../src/support/test";
import {
	createVerifiedAccount,
	PASSWORD,
	signIn,
	uniqueEmail,
} from "../src/support/users";

async function invite(page: import("@playwright/test").Page, email: string) {
	await page.getByRole("link", { name: "Members" }).click();
	await page.getByLabel("Email address").fill(email);
	await page.getByRole("button", { name: "Send invitation" }).click();
	await expect(page.getByText("Invitation sent.")).toBeVisible();
}

test.describe("invitations", () => {
	test("an invited user joins and both appear in the members list", async ({
		workspaceUser,
		isolated,
	}) => {
		const owner = await workspaceUser("Invite Co");
		const inviteeEmail = uniqueEmail("invitee");
		await invite(owner.page, inviteeEmail);

		// A different browser: sign up, verify, sign in, then open the emailed invitation.
		const guestContext = await isolated();
		await createVerifiedAccount(guestContext.request, inviteeEmail);
		const guest = await guestContext.newPage();
		await signIn(guest, { email: inviteeEmail, password: PASSWORD });
		await expect(guest).toHaveURL(/\/onboarding$/);

		await guest.goto(await waitForLink(inviteeEmail, "/invite/accept"));
		await expect(guest).toHaveURL(/\/app$/);
		await expect(
			guest.getByRole("heading", { name: owner.workspaceName, level: 1 }),
		).toBeVisible();

		for (const page of [owner.page, guest]) {
			await page.goto("/app/members");
			const team = page.getByRole("listitem").filter({ hasText: "owner" });
			await expect(team.filter({ hasText: owner.account.email })).toBeVisible();
			await expect(
				page
					.getByRole("listitem")
					.filter({ hasText: inviteeEmail })
					.filter({ hasText: "member" }),
			).toBeVisible();
		}
	});

	test("opening the invitation signed out goes through sign-in and back", async ({
		workspaceUser,
		isolated,
	}) => {
		const owner = await workspaceUser("Return Co");
		const inviteeEmail = uniqueEmail("invitee");
		await invite(owner.page, inviteeEmail);

		const guestContext = await isolated();
		const account = await createVerifiedAccount(
			guestContext.request,
			inviteeEmail,
		);
		const guest = await guestContext.newPage();
		await guest.goto(await waitForLink(inviteeEmail, "/invite/accept"));
		await expect(guest).toHaveURL(/\/login\?redirect=/);

		await guest.getByLabel("Email").fill(account.email);
		await guest.getByLabel("Password", { exact: true }).fill(account.password);
		await guest.getByRole("button", { name: "Sign in" }).click();
		await expect(guest).toHaveURL(/\/app$/);
		await expect(
			guest.getByRole("heading", { name: owner.workspaceName, level: 1 }),
		).toBeVisible();
	});
});

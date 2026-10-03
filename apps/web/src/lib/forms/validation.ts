/**
 * Mirrors the server's password policy (authlayer's default rules, enforced by `SignUp` and
 * `ResetPassword`), so the form says what the API will accept instead of failing after the round
 * trip with a message that names no rule. The server stays the authority.
 */
export const MIN_PASSWORD_LENGTH = 12;

export const PASSWORD_HINT = `At least ${MIN_PASSWORD_LENGTH} characters, with upper and lower case letters, a number and a symbol.`;

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const emailError = (email: string): string | undefined =>
	EMAIL.test(email.trim()) ? undefined : "Enter a valid email address.";

export const newPasswordError = (password: string): string | undefined => {
	const meetsPolicy =
		[...password].length >= MIN_PASSWORD_LENGTH &&
		/\p{Lu}/u.test(password) &&
		/\p{Ll}/u.test(password) &&
		/\p{Nd}/u.test(password) &&
		/[^\p{L}\p{Nd}\s]/u.test(password);
	return meetsPolicy ? undefined : PASSWORD_HINT;
};

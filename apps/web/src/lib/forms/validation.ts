export const MIN_PASSWORD_LENGTH = 8;

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const emailError = (email: string): string | undefined =>
	EMAIL.test(email.trim()) ? undefined : "Enter a valid email address.";

export const newPasswordError = (password: string): string | undefined =>
	password.length >= MIN_PASSWORD_LENGTH
		? undefined
		: `Use at least ${MIN_PASSWORD_LENGTH} characters.`;

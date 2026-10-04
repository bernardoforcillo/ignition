import { authClient } from "~/lib/rpc";

export const signUp = (email: string, password: string) =>
	authClient.signUp({ email, password });

export const verifyEmail = (token: string) => authClient.verifyEmail({ token });

export const requestPasswordReset = (email: string) =>
	authClient.requestPasswordReset({ email });

export const resetPassword = (token: string, newPassword: string) =>
	authClient.resetPassword({ token, newPassword });

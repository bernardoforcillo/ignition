import { Code, ConnectError } from "@connectrpc/connect";

/** Where an error happened; a few codes read differently depending on it. */
export type ErrorContext =
	| "login"
	| "workspace"
	| "token"
	| "password"
	| "default";

const GENERIC = "Something went wrong. Please try again.";

/**
 * Turns anything thrown by an RPC into a message that is safe and useful to show. Raw server
 * errors are never displayed except the validation text the API deliberately returns for
 * `invalid_argument` / `failed_precondition`.
 */
export function errorMessage(
	err: unknown,
	context: ErrorContext = "default",
): string {
	const error = ConnectError.from(err);
	// An emailed link (verify, reset, invite) that the server rejects is simply dead.
	if (
		context === "token" &&
		[
			Code.Unauthenticated,
			Code.NotFound,
			Code.InvalidArgument,
			Code.FailedPrecondition,
			Code.PermissionDenied,
		].includes(error.code)
	) {
		return "This link is invalid or has expired. Request a new one.";
	}
	switch (error.code) {
		case Code.Unauthenticated:
			if (context === "password") return "Incorrect password.";
			return context === "login"
				? "Invalid email or password"
				: "Your session expired. Please sign in again.";
		case Code.ResourceExhausted:
			return "Too many attempts, try again later";
		case Code.AlreadyExists:
			return context === "workspace"
				? "That workspace URL is already taken. Try another."
				: "That already exists.";
		case Code.InvalidArgument:
		case Code.FailedPrecondition:
			return error.rawMessage.trim() || GENERIC;
		case Code.PermissionDenied:
			return "You don't have permission to do that.";
		case Code.NotFound:
			return "We couldn't find that. It may have expired or been removed.";
		case Code.Unavailable:
			return "Can't reach the server. Check your connection and try again.";
		default:
			return GENERIC;
	}
}

/** BillingService is only served when billing is configured; these codes mean it is not. */
export function isBillingDisabled(err: unknown): boolean {
	const { code } = ConnectError.from(err);
	return code === Code.Unimplemented || code === Code.NotFound;
}

export function isUnauthenticated(err: unknown): boolean {
	return ConnectError.from(err).code === Code.Unauthenticated;
}

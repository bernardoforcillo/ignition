import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import {
	errorMessage,
	isBillingDisabled,
	isFilesDisabled,
	isPermissionDenied,
} from "./messages";

const err = (code: Code, message = "boom") => new ConnectError(message, code);

describe("errorMessage", () => {
	it("says the credentials are wrong for unauthenticated on the login screen", () => {
		expect(errorMessage(err(Code.Unauthenticated), "login")).toBe(
			"Invalid email or password",
		);
	});

	it("says the session expired for unauthenticated elsewhere", () => {
		expect(errorMessage(err(Code.Unauthenticated))).toMatch(/session expired/i);
	});

	it("calls a permission_denied on the password confirmation an incorrect password", () => {
		expect(errorMessage(err(Code.PermissionDenied), "password")).toBe(
			"Incorrect password.",
		);
		expect(errorMessage(err(Code.PermissionDenied))).toMatch(/permission/i);
	});

	it("asks the user to wait when rate limited", () => {
		expect(errorMessage(err(Code.ResourceExhausted))).toBe(
			"Too many attempts, try again later",
		);
	});

	it("reports a taken slug for already_exists on workspace forms", () => {
		expect(errorMessage(err(Code.AlreadyExists), "workspace")).toMatch(
			/already taken/i,
		);
	});

	it("shows the validation message for invalid_argument and failed_precondition", () => {
		expect(errorMessage(err(Code.InvalidArgument, "password too short"))).toBe(
			"password too short",
		);
		expect(
			errorMessage(err(Code.FailedPrecondition, "transfer ownership first")),
		).toBe("transfer ownership first");
	});

	it("calls a rejected emailed link invalid or expired", () => {
		expect(errorMessage(err(Code.NotFound), "token")).toMatch(
			/invalid or has expired/i,
		);
		expect(errorMessage(err(Code.InvalidArgument, "bad"), "token")).toMatch(
			/invalid or has expired/i,
		);
	});

	it("falls back to a generic message and never leaks the server text", () => {
		const message = errorMessage(err(Code.Internal, "pq: secret detail"));
		expect(message).not.toContain("secret");
		expect(message).toMatch(/something went wrong/i);
	});

	it("handles non-Connect errors", () => {
		expect(errorMessage(new TypeError("Failed to fetch"))).toMatch(
			/something went wrong|reach the server/i,
		);
	});
});

describe("isBillingDisabled", () => {
	it("is true for unimplemented and not_found", () => {
		expect(isBillingDisabled(err(Code.Unimplemented))).toBe(true);
		expect(isBillingDisabled(err(Code.NotFound))).toBe(true);
	});

	it("is false for other failures", () => {
		expect(isBillingDisabled(err(Code.Internal))).toBe(false);
	});
});

describe("errorMessage in the files context", () => {
	it("explains an exhausted quota instead of blaming rate limits", () => {
		expect(errorMessage(err(Code.ResourceExhausted), "files")).toMatch(
			/run out of storage/i,
		);
	});

	it("tells a member without the permission who can help", () => {
		expect(errorMessage(err(Code.PermissionDenied), "files")).toMatch(
			/owner or admin/i,
		);
	});

	it("shows the validation text the API returns for a refused file", () => {
		expect(
			errorMessage(
				err(Code.InvalidArgument, "this file type is not allowed"),
				"files",
			),
		).toBe("this file type is not allowed");
	});
});

describe("file service availability", () => {
	it("treats unimplemented as storage not enabled", () => {
		expect(isFilesDisabled(err(Code.Unimplemented))).toBe(true);
		expect(isFilesDisabled(err(Code.Internal))).toBe(false);
	});

	it("recognizes a permission error", () => {
		expect(isPermissionDenied(err(Code.PermissionDenied))).toBe(true);
		expect(isPermissionDenied(err(Code.NotFound))).toBe(false);
	});
});

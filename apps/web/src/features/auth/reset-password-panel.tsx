import { Alert, Button, TextField } from "@ignition/components";
import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";

import { resetPassword } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { newPasswordError, useFocusFirstInvalid } from "~/lib/forms";

import { AuthLayout } from "./auth-layout";

type Props = { token?: string };
type Errors = { password?: string; confirm?: string };

export function ResetPasswordPanel({ token }: Props) {
	const formRef = useRef<HTMLFormElement>(null);
	const [password, setPassword] = useState("");
	const [confirm, setConfirm] = useState("");
	const [errors, setErrors] = useState<Errors>({});
	useFocusFirstInvalid(formRef, errors);

	const mutation = useMutation({
		mutationFn: () => resetPassword(token ?? "", password),
	});

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const next: Errors = {
			password: newPasswordError(password),
			confirm: confirm === password ? undefined : "Passwords don't match.",
		};
		const invalid = Object.values(next).some(Boolean);
		setErrors(invalid ? next : {});
		if (!invalid) mutation.mutate();
	};

	if (!token) {
		return (
			<AuthLayout
				title="Reset password"
				footer={
					<Link
						to="/forgot-password"
						className="font-medium text-brand-600 underline"
					>
						Request a new link
					</Link>
				}
			>
				<Alert tone="danger">
					This link is missing its token. Open the link from your email again.
				</Alert>
			</AuthLayout>
		);
	}

	if (mutation.isSuccess) {
		return (
			<AuthLayout title="Password updated">
				<div className="space-y-4">
					<Alert tone="success">
						Your password was changed. Sign in again with the new one.
					</Alert>
					<Link
						to="/login"
						className="block rounded-card bg-brand-600 px-4 py-2 text-center text-sm font-medium text-white hover:bg-brand-700"
					>
						Continue to sign in
					</Link>
				</div>
			</AuthLayout>
		);
	}

	return (
		<AuthLayout
			title="Reset password"
			description="Choose a new password for your account."
		>
			<form ref={formRef} onSubmit={onSubmit} noValidate className="space-y-4">
				{mutation.isError ? (
					<Alert tone="danger">
						{errorMessage(mutation.error, "token")}{" "}
						<Link to="/forgot-password" className="font-medium underline">
							Request a new link
						</Link>
					</Alert>
				) : null}
				<TextField
					label="New password"
					type="password"
					revealable
					autoComplete="new-password"
					hint="At least 8 characters."
					value={password}
					onChange={(e) => setPassword(e.target.value)}
					error={errors.password}
				/>
				<TextField
					label="Confirm new password"
					type="password"
					revealable
					autoComplete="new-password"
					value={confirm}
					onChange={(e) => setConfirm(e.target.value)}
					error={errors.confirm}
				/>
				<Button type="submit" loading={mutation.isPending} className="w-full">
					{mutation.isPending ? "Saving…" : "Reset password"}
				</Button>
			</form>
		</AuthLayout>
	);
}

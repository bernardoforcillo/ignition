import { Alert, Button, TextField } from "@ignition/components";
import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";

import { signUp } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import {
	emailError,
	newPasswordError,
	useFocusFirstInvalid,
} from "~/lib/forms";

import { AuthLayout } from "./auth-layout";

type Errors = { email?: string; password?: string };

export function SignupPanel() {
	const formRef = useRef<HTMLFormElement>(null);
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [errors, setErrors] = useState<Errors>({});
	useFocusFirstInvalid(formRef, errors);

	const mutation = useMutation({
		mutationFn: () => signUp(email.trim(), password),
	});

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const next: Errors = {
			email: emailError(email),
			password: newPasswordError(password),
		};
		const invalid = Object.values(next).some(Boolean);
		setErrors(invalid ? next : {});
		if (!invalid) mutation.mutate();
	};

	// Sign-up answers identically whether or not the address is taken, so this state is the same too.
	if (mutation.isSuccess) {
		return (
			<AuthLayout
				title="Check your email"
				description={
					<>
						We sent a verification link to <strong>{email.trim()}</strong>. Open
						it to activate your account.
					</>
				}
				footer={
					<Link to="/login" className="font-medium text-brand-600 underline">
						Back to sign in
					</Link>
				}
			>
				<Alert tone="info">
					Didn't get it? Check your spam folder, or try signing up again in a
					few minutes.
				</Alert>
			</AuthLayout>
		);
	}

	return (
		<AuthLayout
			title="Create account"
			description="Start with your email and a password."
			footer={
				<>
					Already have an account?{" "}
					<Link to="/login" className="font-medium text-brand-600 underline">
						Sign in
					</Link>
				</>
			}
		>
			<form ref={formRef} onSubmit={onSubmit} noValidate className="space-y-4">
				{mutation.isError ? (
					<Alert tone="danger">{errorMessage(mutation.error)}</Alert>
				) : null}
				<TextField
					label="Email"
					type="email"
					autoComplete="email"
					value={email}
					onChange={(e) => setEmail(e.target.value)}
					error={errors.email}
				/>
				<TextField
					label="Password"
					type="password"
					revealable
					autoComplete="new-password"
					hint="At least 8 characters."
					value={password}
					onChange={(e) => setPassword(e.target.value)}
					error={errors.password}
				/>
				<Button type="submit" loading={mutation.isPending} className="w-full">
					{mutation.isPending ? "Creating account…" : "Create account"}
				</Button>
			</form>
		</AuthLayout>
	);
}

import { Alert, Button, TextField } from "@ignition/components";
import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";
import { analytics } from "~/lib/analytics";
import { requestPasswordReset } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { emailError, useFocusFirstInvalid } from "~/lib/forms";

import { AuthLayout } from "./auth-layout";

type Errors = { email?: string };

export function ForgotPasswordPanel() {
	const formRef = useRef<HTMLFormElement>(null);
	const [email, setEmail] = useState("");
	const [errors, setErrors] = useState<Errors>({});
	useFocusFirstInvalid(formRef, errors);

	const mutation = useMutation({
		mutationFn: () => requestPasswordReset(email.trim()),
		onSuccess: () =>
			analytics.track("password_reset_requested", {
				location: "forgot_password",
			}),
	});

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const error = emailError(email);
		setErrors(error ? { email: error } : {});
		if (!error) mutation.mutate();
	};

	const backLink = (
		<Link to="/login" className="font-medium text-brand-600 underline">
			Back to sign in
		</Link>
	);

	// The API answers the same whether or not the address has an account; so does this screen.
	if (mutation.isSuccess) {
		return (
			<AuthLayout title="Check your email" footer={backLink}>
				<Alert tone="success">
					If an account exists for <strong>{email.trim()}</strong>, we sent a
					link to reset the password.
				</Alert>
			</AuthLayout>
		);
	}

	return (
		<AuthLayout
			title="Forgot password"
			description="Enter your email and we'll send you a reset link."
			footer={backLink}
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
				<Button type="submit" loading={mutation.isPending} className="w-full">
					{mutation.isPending ? "Sending…" : "Send reset link"}
				</Button>
			</form>
		</AuthLayout>
	);
}

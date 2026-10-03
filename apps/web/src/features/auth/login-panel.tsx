import { Alert, Button, TextField } from "@ignition/components";
import { useMutation } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";
import { analytics } from "~/lib/analytics";
import { errorMessage } from "~/lib/errors";
import { emailError, useFocusFirstInvalid } from "~/lib/forms";
import { safeRedirect } from "~/lib/redirect";
import { useAuthStore } from "~/stores/auth";

import { AuthLayout } from "./auth-layout";

type Props = { redirectTo?: string };
type Errors = { email?: string; password?: string };

export function LoginPanel({ redirectTo }: Props) {
	const router = useRouter();
	const login = useAuthStore((state) => state.login);
	const formRef = useRef<HTMLFormElement>(null);
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [errors, setErrors] = useState<Errors>({});
	useFocusFirstInvalid(formRef, errors);

	const mutation = useMutation({
		mutationFn: () => login(email.trim(), password),
		onSuccess: () => {
			analytics.track("login_succeeded", { location: "login" });
			router.history.push(safeRedirect(redirectTo));
		},
	});

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const next: Errors = {
			email: emailError(email),
			password: password ? undefined : "Enter your password.",
		};
		const invalid = Object.values(next).some(Boolean);
		setErrors(invalid ? next : {});
		if (!invalid) mutation.mutate();
	};

	return (
		<AuthLayout
			title="Sign in"
			description="Welcome back. Sign in to your account."
			footer={
				<>
					New here?{" "}
					<Link to="/signup" className="font-medium text-brand-600 underline">
						Create account
					</Link>
				</>
			}
		>
			<form ref={formRef} onSubmit={onSubmit} noValidate className="space-y-4">
				{mutation.isError ? (
					<Alert tone="danger">{errorMessage(mutation.error, "login")}</Alert>
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
					autoComplete="current-password"
					value={password}
					onChange={(e) => setPassword(e.target.value)}
					error={errors.password}
				/>
				<div className="text-right text-sm">
					<Link
						to="/forgot-password"
						className="font-medium text-brand-600 underline"
					>
						Forgot password?
					</Link>
				</div>
				<Button type="submit" loading={mutation.isPending} className="w-full">
					{mutation.isPending ? "Signing in…" : "Sign in"}
				</Button>
			</form>
		</AuthLayout>
	);
}

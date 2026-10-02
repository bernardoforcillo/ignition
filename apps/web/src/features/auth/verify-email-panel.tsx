import { Alert, Skeleton } from "@ignition/components";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";

import { verifyEmail } from "~/lib/api";
import { errorMessage } from "~/lib/errors";

import { AuthLayout } from "./auth-layout";

type Props = { token?: string };

/** Redeems the emailed token once on mount (a query, so StrictMode's double render cannot spend it twice). */
export function VerifyEmailPanel({ token }: Props) {
	const query = useQuery({
		queryKey: ["verify-email", token],
		queryFn: () => verifyEmail(token ?? ""),
		enabled: Boolean(token),
		retry: false,
		staleTime: Number.POSITIVE_INFINITY,
	});

	const backLink = (
		<Link to="/login" className="font-medium text-brand-600 underline">
			Back to sign in
		</Link>
	);

	if (!token || query.isError) {
		return (
			<AuthLayout title="Email verification" footer={backLink}>
				<Alert tone="danger">
					{token
						? errorMessage(query.error, "token")
						: "This link is missing its token. Open the link from your email again."}
				</Alert>
			</AuthLayout>
		);
	}

	if (query.isSuccess) {
		return (
			<AuthLayout title="Email verified">
				<div className="space-y-4">
					<Alert tone="success">
						Your email is verified. You can sign in now.
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
		<AuthLayout title="Verifying your email">
			<div role="status" className="space-y-2">
				<span className="sr-only">Verifying your email…</span>
				<Skeleton className="h-4 w-full" />
				<Skeleton className="h-4 w-2/3" />
			</div>
		</AuthLayout>
	);
}

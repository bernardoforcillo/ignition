import { Alert, Button, Card, Dialog, TextField } from "@ignition/components";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";
import { analytics } from "~/lib/analytics";
import { deleteAccount, exportData, meQuery } from "~/lib/api";
import { downloadBytes } from "~/lib/download";
import { errorMessage } from "~/lib/errors";
import { useFocusFirstInvalid } from "~/lib/forms";
import { useAuthStore } from "~/stores/auth";

export function SettingsPanel() {
	const router = useRouter();
	const storedEmail = useAuthStore((state) => state.user?.email);
	const clearSession = useAuthStore((state) => state.clearSession);
	const me = useQuery(meQuery());
	const email = me.data?.email || storedEmail;

	const exporting = useMutation({
		mutationFn: async () => {
			const res = await exportData();
			downloadBytes(
				res.data as Uint8Array<ArrayBuffer>,
				res.filename || "ignition-export.json",
			);
			analytics.track("data_exported", { location: "settings" });
		},
	});

	const [dialogOpen, setDialogOpen] = useState(false);
	const [password, setPassword] = useState("");
	const [fieldError, setFieldError] = useState<{ password?: string }>({});
	const formRef = useRef<HTMLFormElement>(null);
	useFocusFirstInvalid(formRef, fieldError);

	const closeDialog = () => {
		setDialogOpen(false);
		setPassword("");
		setFieldError({});
		removal.reset();
	};

	const removal = useMutation({
		mutationFn: () => deleteAccount(password),
		onSuccess: () => {
			analytics.track("account_deleted", { location: "settings" });
			// The server already revoked every session; just forget them locally.
			clearSession();
			router.navigate({ to: "/login", search: {} });
		},
	});

	const onDelete = (event: FormEvent) => {
		event.preventDefault();
		if (!password) {
			setFieldError({ password: "Enter your password to confirm." });
			return;
		}
		setFieldError({});
		removal.mutate();
	};

	return (
		<div className="space-y-6">
			<h1 className="text-2xl font-semibold">Settings</h1>

			<Card title="Account">
				<dl className="flex flex-wrap justify-between gap-2">
					<dt>Email</dt>
					<dd className="font-medium text-fg">{email ?? "—"}</dd>
				</dl>
			</Card>

			<Card title="Your data">
				<div className="space-y-3">
					<p>Download everything we hold about you as a JSON file.</p>
					{exporting.isError ? (
						<Alert tone="danger">{errorMessage(exporting.error)}</Alert>
					) : null}
					<Button
						variant="secondary"
						loading={exporting.isPending}
						onClick={() => exporting.mutate()}
					>
						Export my data
					</Button>
				</div>
			</Card>

			<Card title="Danger zone">
				<div className="space-y-3">
					<p>
						Deleting your account is permanent. Workspaces you own alone are
						deleted with it.
					</p>
					<Button variant="danger" onClick={() => setDialogOpen(true)}>
						Delete account
					</Button>
				</div>
			</Card>

			<Dialog
				open={dialogOpen}
				onClose={closeDialog}
				title="Delete your account?"
				description="This can't be undone. Enter your password to confirm."
			>
				<form
					ref={formRef}
					onSubmit={onDelete}
					noValidate
					className="space-y-4"
				>
					{removal.isError ? (
						<Alert tone="danger">
							{errorMessage(removal.error, "password")}
						</Alert>
					) : null}
					<TextField
						label="Confirm your password"
						type="password"
						revealable
						autoComplete="current-password"
						value={password}
						onChange={(e) => setPassword(e.target.value)}
						error={fieldError.password}
					/>
					<div className="flex justify-end gap-2">
						<Button type="button" variant="secondary" onClick={closeDialog}>
							Cancel
						</Button>
						<Button type="submit" variant="danger" loading={removal.isPending}>
							Delete my account
						</Button>
					</div>
				</form>
			</Dialog>
		</div>
	);
}

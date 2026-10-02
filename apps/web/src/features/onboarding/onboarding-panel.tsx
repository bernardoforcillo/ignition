import { Alert, Button, TextField } from "@ignition/components";
import { useMutation } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";

import { createWorkspace, refreshWorkspaces } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { useFocusFirstInvalid } from "~/lib/forms";
import { slugify } from "~/lib/slug";
import { signOut, useAuthStore } from "~/stores/auth";

import { AuthLayout } from "../auth";

type Errors = { name?: string; slug?: string };

export function OnboardingPanel() {
	const router = useRouter();
	const setWorkspaceId = useAuthStore((state) => state.setWorkspaceId);
	const formRef = useRef<HTMLFormElement>(null);
	const [name, setName] = useState("");
	const [slug, setSlug] = useState("");
	// Once the user edits the slug by hand, stop overwriting it from the name.
	const [slugEdited, setSlugEdited] = useState(false);
	const [errors, setErrors] = useState<Errors>({});
	useFocusFirstInvalid(formRef, errors);

	const mutation = useMutation({
		mutationFn: async () => {
			const workspace = await createWorkspace(name.trim(), slug);
			await refreshWorkspaces();
			return workspace;
		},
		onSuccess: (workspace) => {
			setWorkspaceId(workspace.id);
			router.history.push("/app");
		},
	});

	const onNameChange = (value: string) => {
		setName(value);
		if (!slugEdited) setSlug(slugify(value));
	};

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const next: Errors = {
			name: name.trim() ? undefined : "Give your workspace a name.",
			slug: slug ? undefined : "Enter a URL for your workspace.",
		};
		const invalid = Object.values(next).some(Boolean);
		setErrors(invalid ? next : {});
		if (!invalid) mutation.mutate();
	};

	const slugTaken = mutation.isError
		? errorMessage(mutation.error, "workspace")
		: null;

	return (
		<AuthLayout
			title="Create your workspace"
			description="A workspace is where your team and billing live. You can invite people next."
			footer={
				<button
					type="button"
					onClick={async () => {
						await signOut();
						router.history.push("/login");
					}}
					className="font-medium text-brand-600 underline"
				>
					Sign out
				</button>
			}
		>
			<form ref={formRef} onSubmit={onSubmit} noValidate className="space-y-4">
				{slugTaken ? <Alert tone="danger">{slugTaken}</Alert> : null}
				<TextField
					label="Workspace name"
					autoComplete="organization"
					value={name}
					onChange={(e) => onNameChange(e.target.value)}
					error={errors.name}
				/>
				<TextField
					label="Workspace URL"
					autoComplete="off"
					hint="Lowercase letters, numbers and hyphens."
					value={slug}
					onChange={(e) => {
						setSlugEdited(true);
						setSlug(slugify(e.target.value));
					}}
					error={errors.slug}
				/>
				<Button type="submit" loading={mutation.isPending} className="w-full">
					{mutation.isPending ? "Creating…" : "Create workspace"}
				</Button>
			</form>
		</AuthLayout>
	);
}

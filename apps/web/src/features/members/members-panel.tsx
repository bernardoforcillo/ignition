import {
	Alert,
	Badge,
	Button,
	Card,
	EmptyState,
	SelectField,
	Skeleton,
	TextField,
} from "@ignition/components";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useRef, useState } from "react";
import { useCurrentWorkspace } from "~/features/workspace";
import { analytics } from "~/lib/analytics";
import { inviteMember, membersQuery } from "~/lib/api";
import { errorMessage } from "~/lib/errors";
import { emailError, useFocusFirstInvalid } from "~/lib/forms";
import { useAuthStore } from "~/stores/auth";

const ROLES = [
	{ value: "member", label: "Member" },
	{ value: "admin", label: "Admin" },
] as const;

type Errors = { email?: string };

export function MembersPanel() {
	const { workspace } = useCurrentWorkspace();
	const workspaceId = workspace?.id ?? "";
	const me = useAuthStore((state) => state.user);
	const queryClient = useQueryClient();

	const members = useQuery({
		...membersQuery(workspaceId),
		enabled: Boolean(workspaceId),
	});

	const formRef = useRef<HTMLFormElement>(null);
	const [email, setEmail] = useState("");
	const [roleKey, setRoleKey] = useState<string>("member");
	const [errors, setErrors] = useState<Errors>({});
	const [sent, setSent] = useState<Array<{ id: string; email: string }>>([]);
	useFocusFirstInvalid(formRef, errors);

	const invite = useMutation({
		mutationFn: () => inviteMember(workspaceId, email.trim(), roleKey),
		onSuccess: (res) => {
			analytics.track("invitation_sent", {
				location: "members",
				role_key: roleKey,
			});
			setSent((list) => [
				{ id: res.invitation?.id ?? email, email: email.trim() },
				...list,
			]);
			setEmail("");
			queryClient.invalidateQueries({
				queryKey: membersQuery(workspaceId).queryKey,
			});
		},
	});

	const onSubmit = (event: FormEvent) => {
		event.preventDefault();
		const error = emailError(email);
		setErrors(error ? { email: error } : {});
		if (!error) invite.mutate();
	};

	return (
		<div className="space-y-6">
			<h1 className="text-2xl font-semibold">Members</h1>

			<Card title="Team">
				{members.isLoading ? (
					<div className="space-y-2" role="status">
						<span className="sr-only">Loading members…</span>
						<Skeleton className="h-10 w-full" />
						<Skeleton className="h-10 w-full" />
					</div>
				) : members.isError ? (
					<Alert tone="danger">{errorMessage(members.error)}</Alert>
				) : members.data && members.data.length > 0 ? (
					<ul className="divide-y divide-line">
						{members.data.map((member) => {
							const isMe = member.userId === me?.id;
							return (
								<li
									key={member.userId}
									className="flex items-center justify-between gap-3 py-2.5"
								>
									<div className="min-w-0">
										<p className="truncate font-medium text-fg">
											{isMe && me?.email ? me.email : member.userId}
										</p>
										{isMe ? <p className="text-xs">You</p> : null}
									</div>
									<Badge
										tone={member.roleKey === "owner" ? "success" : "neutral"}
									>
										{member.roleKey}
									</Badge>
								</li>
							);
						})}
					</ul>
				) : (
					<EmptyState
						title="No members yet"
						description="Invite someone to collaborate."
					/>
				)}
			</Card>

			<Card title="Invite a teammate">
				<form
					ref={formRef}
					onSubmit={onSubmit}
					noValidate
					className="space-y-4"
				>
					{invite.isError ? (
						<Alert tone="danger">{errorMessage(invite.error)}</Alert>
					) : null}
					{invite.isSuccess ? (
						<Alert tone="success">
							Invitation sent. They'll get an email with a link to join.
						</Alert>
					) : null}
					<div className="grid gap-4 sm:grid-cols-[1fr_10rem]">
						<TextField
							label="Email address"
							type="email"
							autoComplete="off"
							value={email}
							onChange={(e) => setEmail(e.target.value)}
							error={errors.email}
						/>
						<SelectField
							label="Role"
							value={roleKey}
							onChange={(e) => setRoleKey(e.target.value)}
							options={ROLES}
						/>
					</div>
					<Button type="submit" loading={invite.isPending}>
						{invite.isPending ? "Sending…" : "Send invitation"}
					</Button>
				</form>
				{sent.length > 0 ? (
					<div className="mt-5">
						<h3 className="mb-1 text-sm font-semibold text-fg">
							Invited this session
						</h3>
						<ul className="space-y-1">
							{sent.map((item) => (
								<li key={item.id}>{item.email}</li>
							))}
						</ul>
					</div>
				) : null}
			</Card>
		</div>
	);
}

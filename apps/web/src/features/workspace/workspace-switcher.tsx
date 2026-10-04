import { useCurrentWorkspace } from "./use-current-workspace";

export function WorkspaceSwitcher() {
	const { memberships, workspace, select } = useCurrentWorkspace();
	if (!workspace) return null;

	return (
		<div className="flex items-center gap-2">
			<label htmlFor="workspace-switcher" className="sr-only">
				Workspace
			</label>
			<select
				id="workspace-switcher"
				value={workspace.id}
				onChange={(e) => select(e.target.value)}
				className="max-w-48 rounded-card border border-line bg-surface px-2 py-1.5 text-sm font-medium text-fg focus-visible:outline-2 focus-visible:outline-brand-500"
			>
				{memberships.map((m) =>
					m.workspace ? (
						<option key={m.workspace.id} value={m.workspace.id}>
							{m.workspace.name}
						</option>
					) : null,
				)}
			</select>
		</div>
	);
}

import { Component, type ErrorInfo, type ReactNode, useEffect } from "react";

import { analytics } from "~/lib/analytics";

/** What the visitor sees when something throws. It never shows the error text. */
export function ErrorFallback() {
	return (
		<main
			className="mx-auto max-w-md space-y-3 px-4 py-24 text-center"
			role="alert"
		>
			<h1 className="text-xl font-semibold text-fg">Something went wrong</h1>
			<p className="text-sm text-fg-muted">
				The error has been reported. Reload the page to try again.
			</p>
			<button
				type="button"
				onClick={() => window.location.reload()}
				className="rounded-card bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-2 focus-visible:outline-brand-500"
			>
				Reload
			</button>
		</main>
	);
}

/** Router `defaultErrorComponent`: reports the error once, then renders the fallback. */
export function RouteError({ error }: { error: unknown }) {
	useEffect(() => {
		analytics.captureException(error, { source: "route" });
	}, [error]);
	return <ErrorFallback />;
}

interface State {
	failed: boolean;
}

/** Root boundary for errors outside the router (providers, layout). */
export class AppErrorBoundary extends Component<
	{ children: ReactNode },
	State
> {
	state: State = { failed: false };

	static getDerivedStateFromError(): State {
		return { failed: true };
	}

	componentDidCatch(error: Error, info: ErrorInfo): void {
		analytics.captureException(error, {
			source: "boundary",
			component_stack: info.componentStack ?? "",
		});
	}

	render(): ReactNode {
		return this.state.failed ? <ErrorFallback /> : this.props.children;
	}
}

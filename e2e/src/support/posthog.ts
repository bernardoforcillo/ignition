import { env } from "../harness/env";
import type { Capture } from "../harness/fake-posthog";

export type { Capture };

export async function captures(): Promise<Capture[]> {
	return (await (
		await fetch(`${env.posthogUrl}/__captures`)
	).json()) as Capture[];
}

export async function resetCaptures(): Promise<void> {
	await fetch(`${env.posthogUrl}/__reset`);
}

/** Everything one request revealed: its URL (path and query) plus its decoded body. */
export const wire = (capture: Capture): string =>
	`${capture.url}\n${capture.body}`;

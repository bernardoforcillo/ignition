export interface GreetingResponse {
	message: string;
	/** ISO timestamp of when the "server" produced the response. */
	fetchedAt: string;
}

const GREETINGS = [
	"Ignition is up and running.",
	"All systems nominal.",
	"Workspace wired and ready.",
];

/**
 * Stand-in transport call. There is no backend wired into this scaffold yet (`apps/gateway` is a
 * separate, unrelated Go service) — this simulates network latency so the TanStack Query wiring
 * on top of it (loading/error/success states, refetch) is exercised for real rather than against
 * an instantly-resolved promise.
 */
export function fetchGreeting(): Promise<GreetingResponse> {
	return new Promise((resolve) => {
		setTimeout(() => {
			const message = GREETINGS[Math.floor(Math.random() * GREETINGS.length)];
			resolve({ message, fetchedAt: new Date().toISOString() });
		}, 600);
	});
}

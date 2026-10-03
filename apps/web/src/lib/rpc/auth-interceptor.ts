import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";

/** What the interceptor needs from the auth state, kept abstract so it is testable and cycle-free. */
export interface Session {
	getAccessToken(): string | null;
	/** Gets a new access token. Resolves `false` when the session could not be renewed. */
	refresh(): Promise<boolean>;
}

/** AuthService carries its own credentials (password, refresh token): never retry it. */
const AUTH_SERVICE = "saas.v1.AuthService";

/** The one AuthService call the gateway also wants a bearer token on (everything else there is public). */
const BEARER_AUTH_METHOD = "Logout";

/**
 * Attaches `Authorization: Bearer <access token>` and, when the server answers
 * `unauthenticated`, refreshes the session ONCE and replays the call with the new token. A second
 * failure is surfaced as is, so a revoked session can never loop. Streaming calls are not
 * replayed (their request body is consumed).
 */
export function createAuthInterceptor(session: Session): Interceptor {
	return (next) => async (req) => {
		const token = session.getAccessToken();
		if (req.service.typeName === AUTH_SERVICE) {
			// Without the bearer, Logout is answered 401 and the refresh token is never revoked. It is
			// not replayed: the auth store retries it itself with the rotated refresh token.
			if (token && req.method.name === BEARER_AUTH_METHOD) {
				req.header.set("Authorization", `Bearer ${token}`);
			}
			return next(req);
		}

		if (token) req.header.set("Authorization", `Bearer ${token}`);

		try {
			return await next(req);
		} catch (err) {
			const unauthenticated =
				ConnectError.from(err).code === Code.Unauthenticated;
			if (!token || !unauthenticated || req.stream) throw err;

			if (!(await session.refresh())) throw err;
			const renewed = session.getAccessToken();
			if (!renewed) throw err;

			const header = new Headers(req.header);
			header.set("Authorization", `Bearer ${renewed}`);
			return next({ ...req, header });
		}
	};
}

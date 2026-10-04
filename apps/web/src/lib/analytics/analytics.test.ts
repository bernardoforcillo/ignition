import { describe, expect, it, vi } from "vitest";

import { type AnalyticsClient, createAnalytics } from "./analytics";
import { scrubProperties, scrubUrl } from "./scrub";

function fakeClient(): AnalyticsClient & { calls: [string, unknown[]][] } {
	const calls: [string, unknown[]][] = [];
	const rec =
		(name: string) =>
		(...args: unknown[]) => {
			calls.push([name, args]);
		};
	return {
		calls,
		capture: rec("capture"),
		captureException: rec("captureException"),
		identify: rec("identify"),
		reset: rec("reset"),
		optIn: rec("optIn"),
		optOut: rec("optOut"),
		isFeatureEnabled: (key) => key === "on",
		onFeatureFlags: (cb) => {
			cb();
			return () => {};
		},
	};
}

describe("createAnalytics", () => {
	it("is a harmless no-op when no client is configured", () => {
		const a = createAnalytics(null);

		expect(a.enabled).toBe(false);
		expect(() => {
			a.track("login_succeeded", { location: "login" });
			a.pageview("/x");
			a.identify("u1");
			a.reset();
			a.captureException(new Error("boom"));
			a.applyConsent("granted");
		}).not.toThrow();
		expect(a.isFeatureEnabled("on")).toBe(false);
	});

	it("forwards typed events with their properties", () => {
		const c = fakeClient();

		createAnalytics(c).track("invitation_sent", {
			location: "members",
			role_key: "admin",
		});

		expect(c.calls).toEqual([
			[
				"capture",
				["invitation_sent", { location: "members", role_key: "admin" }],
			],
		]);
	});

	it("sends pageviews with the path only", () => {
		const c = fakeClient();

		createAnalytics(c).pageview("/app/billing");

		expect(c.calls).toEqual([
			["capture", ["$pageview", { $pathname: "/app/billing" }]],
		]);
	});

	it("never identifies with an empty id", () => {
		const c = fakeClient();

		createAnalytics(c).identify("");

		expect(c.calls).toEqual([]);
	});

	it("maps consent: granted opts in, denied and unset opt out", () => {
		const c = fakeClient();
		const a = createAnalytics(c);

		a.applyConsent("granted");
		a.applyConsent("denied");
		a.applyConsent("unset");

		expect(c.calls.map(([name]) => name)).toEqual([
			"optIn",
			"optOut",
			"optOut",
		]);
	});

	it("reads feature flags and subscribes to their changes", () => {
		const a = createAnalytics(fakeClient());
		const cb = vi.fn();

		a.onFeatureFlags(cb);

		expect(a.isFeatureEnabled("on")).toBe(true);
		expect(a.isFeatureEnabled("off")).toBe(false);
		expect(cb).toHaveBeenCalledOnce();
	});
});

describe("scrubUrl", () => {
	it.each([
		[
			"https://app.example.com/reset-password?token=abc123",
			"https://app.example.com/reset-password",
		],
		[
			"https://app.example.com/verify-email?token=abc&x=1",
			"https://app.example.com/verify-email?x=1",
		],
		[
			"https://app.example.com/login?redirect=%2Fapp%2Fbilling",
			"https://app.example.com/login",
		],
		[
			"https://app.example.com/invite/accept?token=t#frag",
			"https://app.example.com/invite/accept",
		],
		[
			"https://app.example.com/app/billing?tab=plans",
			"https://app.example.com/app/billing?tab=plans",
		],
		["/reset-password?token=abc", "/reset-password"],
		["/app?x=1#section", "/app?x=1"],
	])("%s -> %s", (input, want) => {
		expect(scrubUrl(input)).toBe(want);
	});

	it("returns an empty string for a value that is not a URL at all", () => {
		expect(scrubUrl("http://")).toBe("");
	});
});

describe("scrubProperties", () => {
	it("scrubs every URL property PostHog fills and leaves the rest alone", () => {
		const out = scrubProperties({
			$current_url: "https://a.example/reset-password?token=SECRET",
			$referrer: "https://a.example/login?redirect=%2Fx",
			$session_entry_url: "https://a.example/invite/accept?token=SECRET2",
			plan: "pro",
			count: 3,
		});

		expect(JSON.stringify(out)).not.toContain("SECRET");
		expect(out).toMatchObject({ plan: "pro", count: 3 });
		expect(out?.$referrer).toBe("https://a.example/login");
	});

	it("passes undefined through", () => {
		expect(scrubProperties(undefined)).toBeUndefined();
	});
});

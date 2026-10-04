import { Link } from "@tanstack/react-router";

import { useAuthStore } from "~/stores/auth";

import { linkButtonClass } from "./link-button";

type Props = {
	/** Label for visitors who are not signed in; signed-in visitors always see "Open app". */
	label?: string;
	variant?: "primary" | "secondary";
	className?: string;
};

/** "Get started" for visitors, "Open app" for people who are already signed in. */
export function PrimaryCta({
	label = "Get started",
	variant = "primary",
	className = "",
}: Props) {
	const signedIn = useAuthStore((state) => state.status === "authenticated");
	return (
		<Link
			to={signedIn ? "/app" : "/signup"}
			className={`${linkButtonClass(variant)} ${className}`}
		>
			{signedIn ? "Open app" : label}
		</Link>
	);
}

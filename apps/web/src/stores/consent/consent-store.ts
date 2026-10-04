import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { ConsentDecision } from "~/lib/analytics";

interface ConsentState {
	decision: ConsentDecision;
	grant: () => void;
	deny: () => void;
}

/**
 * The visitor's analytics choice, persisted so the banner is asked once. "unset" means not asked
 * yet: analytics stays off until the visitor accepts.
 */
export const useConsentStore = create<ConsentState>()(
	persist(
		(set) => ({
			decision: "unset",
			grant: () => set({ decision: "granted" }),
			deny: () => set({ decision: "denied" }),
		}),
		{ name: "ignition-consent" },
	),
);

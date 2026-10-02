import { describe, expect, it, vi } from "vitest";

import { downloadBytes, safeFilename } from "./download-bytes";

const fakeDeps = () => {
	const anchor = { href: "", download: "", click: vi.fn() };
	const blobs: Blob[] = [];
	return {
		anchor,
		blobs,
		deps: {
			createObjectURL: (blob: Blob) => {
				blobs.push(blob);
				return "blob:fake";
			},
			revokeObjectURL: vi.fn(),
			createAnchor: () => anchor,
		},
	};
};

describe("downloadBytes", () => {
	it("clicks an anchor pointing at a blob of the bytes, then revokes the URL", async () => {
		const { anchor, blobs, deps } = fakeDeps();
		downloadBytes(
			new TextEncoder().encode('{"a":1}'),
			"export.json",
			"application/json",
			deps,
		);

		expect(anchor.href).toBe("blob:fake");
		expect(anchor.download).toBe("export.json");
		expect(anchor.click).toHaveBeenCalledOnce();
		expect(blobs[0]?.type).toBe("application/json");
		expect(await blobs[0]?.text()).toBe('{"a":1}');
		expect(deps.revokeObjectURL).toHaveBeenCalledWith("blob:fake");
	});

	it("revokes the URL even when the click throws", () => {
		const { anchor, deps } = fakeDeps();
		anchor.click.mockImplementation(() => {
			throw new Error("blocked");
		});
		expect(() =>
			downloadBytes(new Uint8Array(), "x.json", "application/json", deps),
		).toThrow("blocked");
		expect(deps.revokeObjectURL).toHaveBeenCalled();
	});

	it("never lets the suggested filename carry a path", () => {
		const { anchor, deps } = fakeDeps();
		downloadBytes(new Uint8Array(), "../../etc/passwd", "text/plain", deps);
		expect(anchor.download).toBe("passwd");
	});
});

describe("safeFilename", () => {
	it("falls back when nothing usable is left", () => {
		expect(safeFilename("", "export.json")).toBe("export.json");
		expect(safeFilename("...", "export.json")).toBe("export.json");
	});
});

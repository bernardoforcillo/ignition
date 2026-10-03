import { isValidElement, type ReactNode } from "react";

/** The plain text inside a rendered node, used for heading anchors and the copy button. */
export function nodeText(node: ReactNode): string {
	if (typeof node === "string" || typeof node === "number") return String(node);
	if (Array.isArray(node)) return node.map(nodeText).join("");
	if (isValidElement<{ children?: ReactNode }>(node)) {
		return nodeText(node.props.children);
	}
	return "";
}

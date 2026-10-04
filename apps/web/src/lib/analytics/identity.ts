/** What a change of signed-in account means for analytics. Pure, so it is unit-tested. */
export function identityAction(
	previousId: string,
	nextId: string,
): "identify" | "reset" | "none" {
	if (nextId && nextId !== previousId) return "identify";
	if (!nextId && previousId) return "reset";
	return "none";
}

import { type RefObject, useEffect } from "react";

/** Moves focus to the first field marked `aria-invalid`, so keyboard users land on the error. */
export function focusFirstInvalid(form: HTMLFormElement | null): void {
	form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
}

/** Call `focusFirstInvalid` after each new `errors` object has rendered. */
export function useFocusFirstInvalid(
	formRef: RefObject<HTMLFormElement | null>,
	errors: object,
): void {
	useEffect(() => {
		if (Object.keys(errors).length > 0) focusFirstInvalid(formRef.current);
	}, [errors, formRef]);
}

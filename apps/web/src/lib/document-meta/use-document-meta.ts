import { useEffect } from "react";

type Meta = { title: string; description: string };

const descriptionTag = (): HTMLMetaElement => {
	let tag = document.querySelector<HTMLMetaElement>('meta[name="description"]');
	if (!tag) {
		tag = document.createElement("meta");
		tag.name = "description";
		document.head.append(tag);
	}
	return tag;
};

/**
 * Sets the page title and meta description for the route that renders it and puts the previous
 * values back when the route unmounts, so screens that set nothing never inherit a stale title.
 */
export function useDocumentMeta({ title, description }: Meta): void {
	useEffect(() => {
		const tag = descriptionTag();
		const previousTitle = document.title;
		const previousDescription = tag.content;
		document.title = title;
		tag.content = description;
		return () => {
			document.title = previousTitle;
			tag.content = previousDescription;
		};
	}, [title, description]);
}

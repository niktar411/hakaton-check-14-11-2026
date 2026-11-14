import DOMPurify from "dompurify";
import { marked } from "marked";
import { createEffect, createSignal, onCleanup } from "solid-js";
import s from "./Markdown.module.css";

DOMPurify.addHook("afterSanitizeAttributes", (node) => {
	if (node.tagName === "A") {
		node.setAttribute("target", "_blank");
		node.setAttribute("rel", "noopener noreferrer");
	}
});

function renderMarkdown(content: string) {
	const raw = marked.parse(content, {
		async: false,
		breaks: true,
		gfm: true,
	}) as string;
	return DOMPurify.sanitize(raw, {
		USE_PROFILES: { html: true },
		FORBID_TAGS: ["img", "iframe", "style", "form", "input", "button", "svg"],
		FORBID_ATTR: ["style", "onerror", "onload"],
	});
}

export function Markdown(props: { content: string }) {
	const [html, setHtml] = createSignal("");
	createEffect(() => {
		const content = props.content;
		const timer = window.setTimeout(() => setHtml(renderMarkdown(content)), 120);
		onCleanup(() => window.clearTimeout(timer));
	});
	// The HTML is sanitized with DOMPurify before it reaches the DOM.
	return <div class={s.markdown} innerHTML={html()} />;
}

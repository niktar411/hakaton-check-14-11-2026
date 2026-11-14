import { For, type JSX } from "solid-js";
import s from "./SamPage.module.css";

export type SamTone = "neutral" | "info" | "warning" | "danger" | "success";

export function SamPageHeader(props: { title: string; lead: string }) {
	return (
		<header class={s.secBand}>
			<h1 class={s.secTitle}>{props.title}</h1>
			<p class={s.secLead}>{props.lead}</p>
		</header>
	);
}

export function SamBadge(props: { label: string; tone?: SamTone }) {
	return (
		<span
			class={s.badge}
			classList={{
				[s.badgeInfo]: props.tone === "info",
				[s.badgeWarning]: props.tone === "warning",
				[s.badgeDanger]: props.tone === "danger",
				[s.badgeSuccess]: props.tone === "success",
			}}
		>
			{props.label}
		</span>
	);
}

export function SamFields(props: {
	items: Array<{ label: string; value: string }>;
}) {
	return (
		<dl class={s.fields}>
			<For each={props.items}>
				{(item) => (
					<div class={s.field}>
						<dt>{item.label}</dt>
						<dd>{item.value}</dd>
					</div>
				)}
			</For>
		</dl>
	);
}

export function SamList(props: { children: JSX.Element }) {
	return <ul class={s.list}>{props.children}</ul>;
}

import { A } from "@solidjs/router";
import { Show } from "solid-js";
import * as m from "@/paraglide/messages";
import s from "./Footer.module.css";

export function Footer(props: { plannerUrl?: string; publicUrl?: string }) {
	return (
		<footer class={s.root}>
			<nav class={s.links} aria-label={m.footerNavigation()}>
				<a href={props.publicUrl ?? "/"}>{m.footerHome()}</a>
				<A href="/questions">{m.questionsPageTitle()}</A>
				<A href="/planner">{m.plannerPageTitle()}</A>
				<Show when={props.plannerUrl}>
					{(url) => (
						<a href={url()} target="_blank" rel="noopener noreferrer">
							{m.openPlanner()}
						</a>
					)}
				</Show>
			</nav>
			<p class={s.note}>{m.publicFooterNote()}</p>
		</footer>
	);
}

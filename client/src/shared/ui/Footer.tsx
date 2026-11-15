import { A } from "@solidjs/router";
import { Show } from "solid-js";
import * as m from "@/paraglide/messages";
import s from "./Footer.module.css";

export function Footer(props: {
	plannerUrl?: string;
	publicUrl?: string;
	team?: boolean;
	borderless?: boolean;
}) {
	return (
		<footer class={s.root} data-borderless={props.borderless}>
			<nav class={s.links} aria-label={m.footerNavigation()}>
				<Show
					when={!props.team}
					fallback={
						<>
							<A href="/planner">{m.plannerPageTitle()}</A>
							<A href="/chat">{m.aiChatTitle()}</A>
						</>
					}
				>
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
				</Show>
			</nav>
			<p class={s.note}>
				{props.team ? m.teamFooterNote() : m.publicFooterNote()}
			</p>
		</footer>
	);
}

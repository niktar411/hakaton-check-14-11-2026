import { A } from "@solidjs/router";
import { Show } from "solid-js";
import * as m from "@/paraglide/messages";
import s from "./Footer.module.css";

export function Footer(props: {
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
					<A href="/">{m.teamPageTitle()}</A>
				</Show>
			</nav>
			<p class={s.note}>
				{props.team ? m.teamFooterNote() : m.publicFooterNote()}
			</p>
			<Show when={!props.team}>
				<p class={s.credit}>
					<a
						href="https://game-icons.net/1x1/delapouite/dragon-head.html"
						target="_blank"
						rel="noopener noreferrer"
					>
						{m.teamDragonCredit()}
					</a>
				</p>
			</Show>
		</footer>
	);
}

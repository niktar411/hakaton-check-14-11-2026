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
					<A href="/team">{m.teamPageTitle()}</A>
				</Show>
				<A href="/terms">{m.footerTerms()}</A>
				<A href="/policy">{m.footerPolicy()}</A>
				<A href="/consent">{m.footerConsent()}</A>
				<A href="/cookies">{m.footerCookies()}</A>
			</nav>
			<div class={s.contacts}>
				<a href={`mailto:${m.footerEmail()}`}>{m.footerEmail()}</a>
				<a href={`tel:${m.footerPhone().replaceAll(" ", "")}`}>{m.footerPhone()}</a>
			</div>
			<p class={s.note}>
				{props.team ? m.teamFooterNote() : m.publicFooterNote()}
			</p>
		</footer>
	);
}

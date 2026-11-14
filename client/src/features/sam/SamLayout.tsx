import { A } from "@solidjs/router";
import { createSignal, For, Show, type JSX } from "solid-js";
import * as m from "@/paraglide/messages";
import { SamNotifications } from "./SamNotifications";
import { unseenCount } from "./notifications";
import s from "./SamLayout.module.css";

const sections = [
	{ href: "/sam", label: () => m.samNavOverview(), end: true },
	{ href: "/sam/catalog", label: () => m.samNavCatalog() },
	{ href: "/sam/licenses", label: () => m.samNavLicenses() },
	{ href: "/sam/inventory", label: () => m.samNavInventory() },
	{ href: "/sam/compliance", label: () => m.samNavCompliance() },
	{ href: "/sam/matching", label: () => m.samNavMatching() },
	{ href: "/sam/assistant", label: () => m.samNavAssistant() },
];

/** Заголовок блока: цветной номер-чип, заголовок и необязательная подпись. */
export function SamSectionBand(props: {
	num: string;
	title: string;
	meta?: string;
}) {
	return (
		<div class={s.secBand}>
			<span class={s.secNum}>{props.num}</span>
			<h2 class={s.secTitle}>{props.title}</h2>
			{props.meta && <p class={s.secMeta}>{props.meta}</p>}
		</div>
	);
}

/** Shared frame for every SAM screen: section navigation and a single width. */
export function SamLayout(props: { children?: JSX.Element }) {
	const [notificationsOpen, setNotificationsOpen] = createSignal(false);
	return (
		<div classList={{ [s.page]: true, [s.samRoot]: true }} data-sam>
			<div class={s.navRow}>
				<nav class={s.nav} aria-label={m.samSectionNavigation()}>
					<For each={sections}>
						{(section) => (
							<A
								href={section.href}
								end={section.end}
								class={s.link}
								activeClass={s.linkActive}
							>
								{section.label()}
							</A>
						)}
					</For>
				</nav>
				<div class={s.navActions}>
					<button type="button" class={s.printBtn} onClick={() => window.print()}>
						{m.samPrint()}
					</button>
					<button
						type="button"
						class={s.bellBtn}
						classList={{ [s.bellActive]: notificationsOpen() }}
						aria-label={m.samNotifications()}
						aria-expanded={notificationsOpen()}
						onClick={() => setNotificationsOpen(!notificationsOpen())}
					>
						<svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none">
							<path
								d="M12 3a6 6 0 0 0-6 6v3.3c0 .5-.2 1-.5 1.4L4.2 15.6c-.5.7 0 1.7.9 1.7h13.8c.9 0 1.4-1 .9-1.7l-1.3-1.9a2.5 2.5 0 0 1-.5-1.4V9a6 6 0 0 0-6-6Z"
								stroke="currentColor"
								stroke-width="1.7"
								stroke-linejoin="round"
							/>
							<path
								d="M9.8 20a2.3 2.3 0 0 0 4.4 0"
								stroke="currentColor"
								stroke-width="1.7"
								stroke-linecap="round"
							/>
						</svg>
						<Show when={unseenCount() > 0}>
							<span class={s.bellBadge}>{unseenCount()}</span>
						</Show>
					</button>
				</div>
			</div>
			<SamNotifications open={notificationsOpen()} />
			<main>{props.children}</main>
		</div>
	);
}

/**
 * Page header: title, lead and a compact meta line (дата · источник · числа).
 */
export function SamDocHeader(props: {
	title: string;
	lead: string;
	meta: { label: string; value: string }[];
}) {
	return (
		<header class={s.docHead}>
			<h1 class={s.docTitle}>{props.title}</h1>
			<p class={s.docLead}>{props.lead}</p>
			<p class={s.docMetaLine}>
				<For each={props.meta}>
					{(item, index) => (
						<>
							{index() > 0 && <span class={s.docMetaDot} aria-hidden="true">·</span>}
							<span class={s.docMetaItem}>
								{item.label}: <strong>{item.value}</strong>
							</span>
						</>
					)}
				</For>
			</p>
		</header>
	);
}

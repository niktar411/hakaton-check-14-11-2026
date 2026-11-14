import { A } from "@solidjs/router";
import { For, type JSX } from "solid-js";
import * as m from "@/paraglide/messages";
import { SamNotifications } from "./SamNotifications";
import s from "./SamLayout.module.css";

const sections = [
	{ href: "/sam", label: () => m.samDashboardPageTitle(), end: true },
	{ href: "/sam/catalog", label: () => m.samCatalogPageTitle() },
	{ href: "/sam/licenses", label: () => m.samLicensesPageTitle() },
	{ href: "/sam/inventory", label: () => m.samInventoryPageTitle() },
	{ href: "/sam/compliance", label: () => m.samCompliancePageTitle() },
	{ href: "/sam/matching", label: () => m.samMatchingPageTitle() },
	{ href: "/sam/assistant", label: () => m.samAssistantPageTitle() },
];

/** Чёрная полоса-заголовок раздела с номером и выходом за левую границу. */
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
	return (
		<div classList={{ [s.page]: true, [s.samRoot]: true }}>
			<aside class={s.disclaimer}>
				<strong>{m.samDemoDisclaimer()}</strong>
				<span>{m.samDemoDisclaimerDetails()}</span>
			</aside>
			<SamNotifications />
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
			<main>{props.children}</main>
		</div>
	);
}

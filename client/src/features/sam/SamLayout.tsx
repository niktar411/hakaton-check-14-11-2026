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

/** Shared frame for every SAM screen: section navigation and a single width. */
export function SamLayout(props: { children?: JSX.Element }) {
	return (
		<div class={s.page}>
			<aside class={s.disclaimer} data-tone="warning">
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
			{props.children}
		</div>
	);
}

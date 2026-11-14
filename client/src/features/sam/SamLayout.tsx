import { A, Outlet } from "@solidjs/router";
import { For } from "solid-js";
import * as m from "@/paraglide/messages";
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
export function SamLayout() {
	return (
		<div class={s.page}>
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
			<Outlet />
		</div>
	);
}


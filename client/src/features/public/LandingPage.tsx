import { A } from "@solidjs/router";
import { For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { useSiteConfig } from "@/app/site-config";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import s from "./LandingPage.module.css";

const capabilities = [
	{
		title: () => m.landingQuestionsTitle(),
		description: () => m.landingQuestionsDescription(),
		href: "/questions",
		action: () => m.questionsPageTitle(),
	},
	{
		title: () => m.landingPlannerTitle(),
		description: () => m.landingPlannerDescription(),
		href: "/planner",
		action: () => m.plannerPageTitle(),
	},
];

export default function LandingPage() {
	const { config, retry } = useSiteConfig();
	return (
		<main class={s.page}>
			<section class={s.hero}>
				<p class={s.eyebrow}>{m.appTitle()}</p>
				<h1>{m.landingPageTitle()}</h1>
				<p class={s.lead}>{m.landingSummary()}</p>
				<div class={s.actions}>
					<A class={s.primaryAction} href="/questions">
						{m.askQuestion()}
					</A>
					<Show when={config()?.plannerBaseUrl}>
						{(url) => (
							<a class={s.secondaryAction} href={url()} target="_blank" rel="noopener noreferrer">
								{m.openPlanner()}
							</a>
						)}
					</Show>
				</div>
			</section>
			<section class={s.section} aria-labelledby="capabilities-title">
				<h2 id="capabilities-title">{m.capabilitiesTitle()}</h2>
				<div class={s.cards}>
					<For each={capabilities}>
						{(item) => (
							<article class={s.card}>
								<h3>{item.title()}</h3>
								<p>{item.description()}</p>
								<A href={item.href}>{item.action()}</A>
							</article>
						)}
					</For>
				</div>
			</section>
			<section class={s.section}>
				<h2>{m.workflowTitle()}</h2>
				<p>{m.workflowDescription()}</p>
			</section>
			<section class={s.section} aria-labelledby="project-links-title">
				<h2 id="project-links-title">{m.projectLinks()}</h2>
				<Show when={!config.loading} fallback={<LoadingState />}>
					<Show when={!config.error} fallback={<ErrorState onRetry={retry} />}>
						<Show when={config()}>
							{(data) => (
								<>
									<a class={s.publicLink} href={data().publicBaseUrl}>
										{m.publicSite()}
									</a>
									<For each={data().links ?? []} fallback={<EmptyState message={m.projectLinksEmpty()} />}>
										{(link) => (
											<a class={s.publicLink} href={link.path} target="_blank" rel="noopener noreferrer">
												{link.title}
											</a>
										)}
									</For>
								</>
							)}
						</Show>
					</Show>
				</Show>
			</section>
		</main>
	);
}

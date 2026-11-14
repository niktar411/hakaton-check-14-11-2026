import { createSignal, For } from "solid-js";
import * as m from "@/paraglide/messages";
import { useSiteConfig } from "@/app/site-config";
import s from "./AdministrationPage.module.css";

type DemoLink = { title: string; url: string; hint: string };
type DemoRole = { name: string; role: string };
type DemoCheck = { id: string; text: string };

const publicFallback = "https://cool-domain.online";
const plannerFallback = "https://planner.cool-domain.online";
const aiUrl = "https://ai.cool-domain.online";
const giteaUrl = "http://10.77.0.1/";
const pullRequestUrl = "http://10.77.0.1/hackathon/hackathon/pulls/4";

export function DemoPanel() {
	const { config } = useSiteConfig();
	const [checked, setChecked] = createSignal<ReadonlySet<string>>(new Set());
	const publicUrl = () => config()?.publicBaseUrl || publicFallback;
	const plannerUrl = () => config()?.plannerBaseUrl || plannerFallback;
	const links = (): DemoLink[] => [
		{
			title: m.demoLinkPublic(),
			url: publicUrl(),
			hint: m.demoLinkPublicHint(),
		},
		{
			title: m.demoLinkPlanner(),
			url: plannerUrl(),
			hint: m.demoLinkPlannerHint(),
		},
		{
			title: m.demoLinkAi(),
			url: aiUrl,
			hint: m.demoLinkAiHint(),
		},
		{
			title: m.demoLinkGitea(),
			url: giteaUrl,
			hint: m.demoLinkGiteaHint(),
		},
		{
			title: m.demoLinkPr(),
			url: pullRequestUrl,
			hint: m.demoLinkPrHint(),
		},
	];
	const roles = (): DemoRole[] => [
		{ name: m.demoMemberEldar(), role: m.demoRoleEldar() },
		{ name: m.demoMemberEgor(), role: m.demoRoleEgor() },
		{ name: m.demoMemberIgor(), role: m.demoRoleIgor() },
		{ name: m.demoMemberLenya(), role: m.demoRoleLenya() },
		{ name: m.demoMemberLead(), role: m.demoRoleLead() },
	];
	const checks = (): DemoCheck[] => [
		{ id: "stand", text: m.demoCheckStand() },
		{ id: "scenario", text: m.demoCheckScenario() },
		{ id: "public", text: m.demoCheckPublic() },
		{ id: "planner", text: m.demoCheckPlanner() },
		{ id: "ai", text: m.demoCheckAi() },
		{ id: "pr", text: m.demoCheckPr() },
		{ id: "accounts", text: m.demoCheckAccounts() },
		{ id: "fallback", text: m.demoCheckFallback() },
	];

	function toggle(id: string) {
		const next = new Set(checked());
		if (next.has(id)) {
			next.delete(id);
		} else {
			next.add(id);
		}
		setChecked(next);
	}

	return (
		<section class={s.panel}>
			<div>
				<h2>{m.adminDemo()}</h2>
				<p class={s.muted}>{m.demoHint()}</p>
			</div>
			<section class={s.section}>
				<h2>{m.demoLinks()}</h2>
				<div class={s.grid}>
					<For each={links()}>
						{(link) => (
							<article class={s.card}>
								<div class={s.cardHeader}>
									<h3>{link.title}</h3>
								</div>
								<p class={s.muted}>{link.hint}</p>
								<a
									href={link.url}
									target="_blank"
									rel="noopener noreferrer"
								>
									{link.url}
								</a>
							</article>
						)}
					</For>
				</div>
			</section>
			<section class={s.section}>
				<h2>{m.demoRoles()}</h2>
				<ul class={s.list}>
					<For each={roles()}>
						{(member) => (
							<li class={s.row}>
								<strong>{member.name}</strong>
								<span>{member.role}</span>
							</li>
						)}
					</For>
				</ul>
			</section>
			<section class={s.section}>
				<h2>{m.demoChecklist()}</h2>
				<ul class={s.list}>
					<For each={checks()}>
						{(item) => (
							<li>
								<button
									type="button"
									class={s.choiceButton}
									data-active={checked().has(item.id)}
									aria-pressed={checked().has(item.id)}
									onClick={() => toggle(item.id)}
								>
									{checked().has(item.id) ? "✓ " : ""}
									{item.text}
								</button>
							</li>
						)}
					</For>
				</ul>
			</section>
		</section>
	);
}

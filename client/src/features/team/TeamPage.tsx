import {
	createMemo,
	createResource,
	createSignal,
	For,
	onCleanup,
	onMount,
	Show,
} from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Dialog } from "@/shared/ui/Dialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { PublicAssistant } from "@/features/questions/PublicAssistant";
import {
	PlannerBoard,
	plannerStatus,
} from "@/features/planner/PlannerPage";
import { DragonScene } from "./DragonScene";
import { team } from "./team-data";
import type { TeamMember } from "./readme-team";
import s from "./TeamPage.module.css";

type PublicBlock = components["schemas"]["PublicPlannerBlock"];

const assistantPoints = [
	() => m.teamAiPointOne(),
	() => m.teamAiPointTwo(),
	() => m.teamAiPointThree(),
	() => m.teamAiPointFour(),
];

function initials(name: string) {
	return name
		.split(/\s+/)
		.slice(0, 2)
		.map((part) => part.slice(0, 1).toUpperCase())
		.join("");
}

export default function TeamPage() {
	const [selected, setSelected] = createSignal<PublicBlock>();
	const [selectedMember, setSelectedMember] = createSignal<TeamMember>();
	const [planner, { refetch }] = createResource(
		async () => unwrap(await api.GET("/api/public/planner")) ?? [],
	);
	const total = createMemo(() => planner()?.length ?? 0);
	const completed = createMemo(
		() =>
			planner()?.filter((block) => block.progress.state === "done").length ?? 0,
	);
	const progress = createMemo(() =>
		total() > 0 ? Math.round((completed() / total()) * 100) : 0,
	);

	onMount(() => {
		const hash = window.location.hash.slice(1);
		if (hash) {
			document.getElementById(hash)?.scrollIntoView({ behavior: "smooth" });
		}
		const timer = window.setInterval(() => {
			if (!planner.loading) void refetch();
		}, 15000);
		const onFocus = () => void refetch();
		window.addEventListener("focus", onFocus);
		onCleanup(() => {
			window.clearInterval(timer);
			window.removeEventListener("focus", onFocus);
		});
	});

	return (
		<main class={s.page}>
			<section class={s.hero} aria-labelledby="team-hero-title">
				<div class={s.backdrop} aria-hidden="true">
					<span class={s.glowOne} />
					<span class={s.glowTwo} />
					<span class={s.grid} />
					<span class={s.ringOuter} />
					<span class={s.ringInner} />
				</div>

				<div class={s.heroCopy}>
					<p class={s.eyebrow}>
						<span class={s.eyebrowDot} aria-hidden="true" />
						{m.teamHeroEyebrow()}
					</p>
					<h1 id="team-hero-title">{team.name ?? m.teamPageTitle()}</h1>
					<Show when={team.tagline}>
						{(tagline) => <p class={s.tagline}>{tagline()}</p>}
					</Show>
					<p class={s.summary}>{m.teamHeroSummary()}</p>
					<div class={s.actions}>
						<a class={s.primaryAction} href="#plan">
							{m.teamHeroCtaPlan()}
						</a>
						<a class={s.secondaryAction} href="#assistant">
							{m.teamHeroCtaAssistant()}
						</a>
					</div>
					<dl class={s.stats}>
						<div>
							<dt>{m.teamStatMembers()}</dt>
							<dd>{team.members.length}</dd>
						</div>
						<div>
							<dt>{m.teamStatDone()}</dt>
							<dd>{planner() ? completed() : "—"}</dd>
						</div>
					</dl>
				</div>

				<DragonScene class={s.dragonScene} />
			</section>

			<section id="plan" class={s.section} aria-labelledby="team-plan-title">
				<header class={s.sectionHeader}>
					<p class={s.sectionLabel}>
						{m.teamPlanLabel()}
					</p>
					<h2 id="team-plan-title">{m.teamPlanTitle()}</h2>
					<p class={s.sectionText}>{m.teamPlanDescription()}</p>
				</header>

				<div class={s.progressCard}>
					<div class={s.progressCopy}>
						<span>{m.teamPlanProgress({ done: completed(), total: total() })}</span>
						<span class={s.progressValue}>{progress()}%</span>
					</div>
					<div
						class={s.progressTrack}
						role="progressbar"
						aria-valuemin={0}
						aria-valuemax={100}
						aria-valuenow={progress()}
					>
						<span
							class={s.progressFill}
							style={{ transform: `scaleX(${progress() / 100})` }}
						/>
					</div>
					<p class={s.progressHint}>{m.plannerPublicHint()}</p>
				</div>

				<Show
					when={!planner.loading || !!planner.latest}
					fallback={<LoadingState />}
				>
					<Show
						when={!planner.error || !!planner.latest}
						fallback={<ErrorState onRetry={() => void refetch()} />}
					>
						<Show when={planner.error}>
							<p class={s.stale} role="status">
								{m.plannerStale()}
							</p>
						</Show>
						<Show
							when={total() > 0}
							fallback={<EmptyState message={m.plannerEmpty()} />}
						>
							<PlannerBoard
								blocks={planner() ?? []}
								team={false}
								contained
								wheelZoom={false}
								onSelect={(block) => {
									if (!("owner" in block)) setSelected(block);
								}}
							/>
						</Show>
					</Show>
				</Show>
			</section>

			<section id="team" class={s.section} aria-labelledby="team-people-title">
				<header class={s.sectionHeader}>
					<p class={s.sectionLabel}>
						{m.teamTeamLabel()}
					</p>
					<h2 id="team-people-title">{m.teamTeamTitle()}</h2>
					<p class={s.sectionText}>
						{team.summary ?? m.teamTeamDescription()}
					</p>
				</header>

				<Show
					when={team.members.length > 0}
					fallback={<EmptyState message={m.teamEmpty()} />}
				>
					<ul class={s.memberGrid}>
						<For each={team.members}>
							{(member, index) => (
								<li
									class={s.memberCard}
									style={{ "animation-delay": `${index() * 70}ms` }}
								>
									<button
										type="button"
										class={s.memberButton}
										onClick={() => setSelectedMember(member)}
										aria-label={`${member.name} — ${m.teamMemberDetails()}`}
									>
										<span class={s.avatar} aria-hidden="true">
											<Show when={member.photo} fallback={initials(member.name)}>
												<img
													class={s.avatarPhoto}
													src={member.photo}
													alt=""
													loading="lazy"
													onError={(event) => {
														event.currentTarget.remove();
													}}
												/>
											</Show>
										</span>
										<h3>{member.name}</h3>
										<p class={s.memberRole}>{member.role}</p>
									</button>
									<Show when={member.telegram}>
										{(url) => (
											<a
												class={s.memberLink}
												href={url()}
												target="_blank"
												rel="noopener noreferrer"
												aria-label={`${m.teamTelegram()}: ${member.name}`}
											>
												{m.teamTelegram()} ↗
											</a>
										)}
									</Show>
								</li>
							)}
						</For>
					</ul>
				</Show>
			</section>

			<section
				id="assistant"
				class={s.section}
				aria-labelledby="team-assistant-title"
			>
				<div class={s.assistantGrid}>
					<div class={s.assistantCopy}>
						<p class={s.sectionLabel}>
							{m.teamAiLabel()}
						</p>
						<h2 id="team-assistant-title">{m.teamAiTitle()}</h2>
						<p class={s.sectionText}>{m.teamAiDescription()}</p>
						<ul class={s.assistantPoints}>
							<For each={assistantPoints}>
								{(point) => <li>{point()}</li>}
							</For>
						</ul>
						<span class={s.assistantOrb} aria-hidden="true">
							<span class={s.assistantOrbCore} />
							<span class={s.assistantOrbRing} />
							<span class={s.assistantOrbRing} />
						</span>
					</div>
					<div class={s.assistantCard}>
						<PublicAssistant />
					</div>
				</div>
			</section>

			<Dialog
				open={!!selected()}
				title={selected()?.title || m.plannerDetails()}
				onClose={() => setSelected(undefined)}
			>
				<Show when={selected()}>
					{(block) => (
						<div class={s.dialogBody}>
							<span class={s.status} data-status={block().progress.state}>
								{plannerStatus(block().progress.state)}
							</span>
							<dl class={s.dialogMeta}>
								<div>
									<dt>{m.plannerUpdated()}</dt>
									<dd>
										<time datetime={block().updatedAt}>
											{formatDate(block().updatedAt)}
										</time>
									</dd>
								</div>
							</dl>
							<p class={s.dialogHint}>{m.plannerPublicHint()}</p>
						</div>
					)}
				</Show>
			</Dialog>

			<Dialog
				open={!!selectedMember()}
				title={selectedMember()?.name ?? ""}
				onClose={() => setSelectedMember(undefined)}
			>
				<Show when={selectedMember()}>
					{(member) => (
						<div class={s.memberDialog}>
							<Show when={member().photo} fallback={<span class={s.avatar}>{initials(member().name)}</span>}>
								{(photo) => (
									<img
										class={s.memberDialogPhoto}
										src={photo()}
										alt=""
									/>
								)}
							</Show>
							<div class={s.memberDialogBody}>
								<p class={s.memberRole}>{member().role}</p>
								<Show when={member().quote}>
									{(quote) => <blockquote class={s.memberQuote}>{quote()}</blockquote>}
								</Show>
								<Show when={member().bio}>
									{(bio) => <p class={s.memberBio}>{bio()}</p>}
								</Show>
								<Show when={member().telegram}>
									{(url) => (
										<a
											class={s.memberLink}
											href={url()}
											target="_blank"
											rel="noopener noreferrer"
										>
											{m.teamTelegram()} ↗
										</a>
									)}
								</Show>
							</div>
						</div>
					)}
				</Show>
			</Dialog>
		</main>
	);
}

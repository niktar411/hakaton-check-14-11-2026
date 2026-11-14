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
import { DragonMark } from "@/shared/ui/DragonMark";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { PublicAssistant } from "@/features/questions/PublicAssistant";
import {
	PlannerBoard,
	plannerStatus,
} from "@/features/planner/PlannerPage";
import { DragonScene } from "./DragonScene";
import type { TeamMember } from "./readme-team";
import { team } from "./team-data";
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

/** First sentence of the bio, used as the short summary in the profile window. */
function summaryOf(member: TeamMember) {
	const bio = member.bio?.trim();
	if (!bio) return member.role;
	const sentence = /^.+?[.!?](?:\s|$)/.exec(bio);
	return sentence ? sentence[0].trim() : bio;
}

function TelegramIcon() {
	return (
		<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
			<path d="M21.9 4.3 2.9 11.6c-1 .4-1 1.8.1 2.1l4.7 1.5 1.8 5.4c.3.9 1.4 1.1 2 .4l2.5-2.6 4.8 3.5c.8.6 1.9.2 2.1-.8l3-14.7c.2-1-.8-1.8-2-1.4Z" />
		</svg>
	);
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
			document.getElementById(hash)?.scrollIntoView();
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
						<span class={s.sectionIndex} aria-hidden="true">
							01
						</span>
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
							style={{ width: `${progress()}%` }}
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
								onSelect={(block) => {
									if (!("owner" in block)) setSelected(block);
								}}
							/>
						</Show>
					</Show>
				</Show>
			</section>

			<section id="team" class={s.section} aria-labelledby="team-people-title">
				<div class={s.teamHeading}>
					<span class={s.teamHeadingLine} aria-hidden="true" />
					<h2 id="team-people-title">{m.teamTeamTitle()}</h2>
					<span class={s.teamHeadingLine} aria-hidden="true" />
				</div>

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
										class={s.memberCardButton}
										aria-haspopup="dialog"
										aria-label={`${member.name}: ${m.teamMemberDetails()}`}
										onClick={() => setSelectedMember(member)}
									>
										<span class={s.memberPhotoFrame}>
											<Show
												when={member.photo}
												fallback={
													<>
														<DragonMark class={s.memberPhotoMark} />
														<span
															class={s.memberPhotoInitials}
															aria-hidden="true"
														>
															{initials(member.name)}
														</span>
													</>
												}
											>
												{(photo) => (
													<img
														class={s.memberPhotoImage}
														src={photo()}
														alt=""
														loading="lazy"
													/>
												)}
											</Show>
										</span>
										<span class={s.memberCardBody}>
											<span class={s.memberCardName}>{member.name}</span>
											<span class={s.memberRole}>{member.role}</span>
										</span>
									</button>
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
							<span class={s.sectionIndex} aria-hidden="true">
								03
							</span>
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
				open={!!selectedMember()}
				title={selectedMember()?.name ?? m.teamPageTitle()}
				hideHeader
				onClose={() => setSelectedMember(undefined)}
			>
				<Show when={selectedMember()}>
					{(member) => (
						<div class={s.memberProfile}>
							<div class={s.memberTop}>
								<div class={s.memberPortraitColumn}>
									<Show
										when={member().photo}
										fallback={
											<span class={s.memberPortraitFallback}>
												<DragonMark class={s.memberPortraitMark} />
												<span>{initials(member().name)}</span>
											</span>
										}
									>
										{(photo) => (
											<img
												class={s.memberPortrait}
												src={photo()}
												alt=""
												loading="lazy"
											/>
										)}
									</Show>
									<Show when={member().telegram}>
										{(url) => (
											<a
												class={s.memberSocial}
												href={url()}
												target="_blank"
												rel="noopener noreferrer"
												aria-label={`${m.teamTelegram()}: ${member().name}`}
											>
												<TelegramIcon />
											</a>
										)}
									</Show>
								</div>

								<div class={s.memberIntro}>
									<p class={s.memberEyebrow}>
										<span class={s.memberEyebrowDot} aria-hidden="true" />
										{m.teamMemberNumber({
											number: String(
												team.members.indexOf(member()) + 1,
											).padStart(2, "0"),
										})}
									</p>
									<h2 class={s.memberProfileName}>{member().name}</h2>
									<p class={s.memberDialogRole}>{member().role}</p>
									<p class={s.memberSummary}>{summaryOf(member())}</p>
								</div>
							</div>

							<Show when={member().quote}>
								{(quote) => (
									<blockquote class={s.memberQuote}>
										<p>«{quote()}»</p>
									</blockquote>
								)}
							</Show>

							<div class={s.memberDivider} />

							<div class={s.memberAbout}>
								<div class={s.memberBio}>
									<h3>{m.teamMemberBio()}</h3>
									<Show when={member().bio}>
										{(bio) => <p>{bio()}</p>}
									</Show>
								</div>

								<div class={s.codeCard} aria-hidden="true">
									<div class={s.codeBar}>
										<span class={s.codeDotRed} />
										<span class={s.codeDotYellow} />
										<span class={s.codeDotGreen} />
										<span class={s.codeFile}>profile.js</span>
									</div>
									<pre class={s.codeBlock}>
										<code>
											<span class={s.codeKeyword}>const</span>{" "}
											<span class={s.codeDeveloper}>developer</span> =
											{" "}
											{"{"}
											{"\n"} <span class={s.codeProperty}>name</span>:{" "}
											<span class={s.codeString}>
												"{member().name}"
											</span>
											,
											{"\n"} <span class={s.codeProperty}>role</span>:{" "}
											<span class={s.codeStringAlt}>
												"{member().role}"
											</span>
											,
											{"\n"} <span class={s.codeProperty}>goal</span>:{" "}
											<span class={s.codeString}>
												"{m.teamMemberGoal()}"
											</span>
											{"\n"}
											{"};"}
										</code>
									</pre>
								</div>
							</div>

							<div class={s.memberDivider} />

							<div class={s.memberDuty}>
								<h3>{m.teamMemberRoleTitle()}</h3>
								<p class={s.memberDutyRole}>{member().role}</p>
							</div>
						</div>
					)}
				</Show>
			</Dialog>

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
		</main>
	);
}

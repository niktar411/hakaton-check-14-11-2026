import { createMemo, createResource, createSignal, For, onMount, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import { Button } from "@/shared/ui/Button";
import { SamBadge } from "./SamPage";
import { findingSeverityLabel, findingSeverityTone } from "./labels";
import { notificationKey, readSeen, writeSeen } from "./notifications";
import s from "./notifications.module.css";

export function SamNotifications() {
	const [findings] = createResource(async () => unwrap(await api.GET("/api/public/sam/compliance")) ?? []);
	const [seen, setSeen] = createSignal<Set<string>>(new Set());

	onMount(() => {
		setSeen(new Set(readSeen()));
	});

	const critical = createMemo(
		() => (findings() ?? []).filter((finding) => finding.severity === "high" || finding.severity === "critical"),
	);
	const unseen = createMemo(() => critical().filter((finding) => !seen().has(notificationKey(finding))));

	function markAll() {
		const next = new Set(seen());
		for (const finding of unseen()) next.add(notificationKey(finding));
		writeSeen([...next]);
		setSeen(next);
	}

	return (
		<section class={s.root} aria-label={m.samNotificationsTitle()}>
			<header class={s.header}>
				<div>
					<h2 class={s.title}>{m.samNotificationsTitle()}</h2>
					<p class={s.lead}>{m.samNotificationsDescription()}</p>
				</div>
				<Show when={unseen().length > 0}>
					<span class={s.count}>{unseen().length}</span>
				</Show>
			</header>
			<Show
				when={unseen().length > 0}
				fallback={<p class={s.empty}>{m.samNotificationsEmpty()}</p>}
			>
				<ul class={s.list}>
					<For each={unseen()}>
						{(finding) => (
							<li class={s.item}>
								<SamBadge
									label={findingSeverityLabel(finding.severity)}
									tone={findingSeverityTone(finding.severity)}
								/>
								<div class={s.body}>
									<strong>{finding.code}</strong>
									<span class={s.meta}>
										{finding.product || m.samNotSpecified()} · {finding.asset || m.samNotSpecified()}
									</span>
									<Show when={finding.details}>
										<span class={s.details}>{finding.details}</span>
									</Show>
								</div>
							</li>
						)}
					</For>
				</ul>
				<Button variant="secondary" onClick={markAll}>
					{m.samNotificationsMarkRead()}
				</Button>
			</Show>
		</section>
	);
}

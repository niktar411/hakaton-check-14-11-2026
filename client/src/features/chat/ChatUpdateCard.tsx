import { For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import type { components } from "@/shared/api/schema";
import { Button } from "@/shared/ui/Button";
import s from "./ChatUpdateCard.module.css";

type ChatUpdate = components["schemas"]["ChatUpdate"];

export function ChatUpdateCard(props: {
	updates: ChatUpdate[];
	busy: boolean;
	onPull: (update: ChatUpdate) => void;
}) {
	return (
		<Show when={props.updates.length > 0}>
			<For each={props.updates}>
				{(update) => (
					<article class={s.card} role="status">
						<header class={s.header}>
							<strong>{m.chatUpdateTitle({ title: update.title })}</strong>
							<span>{m.chatUpdateBy({ owner: update.ownerName })}</span>
						</header>
						<Show when={update.branch}>
							<p class={s.meta}>{m.chatUpdateBranch({ branch: update.branch })}</p>
						</Show>
						<Show when={update.description}>
							<p class={s.description}>{update.description}</p>
						</Show>
						<div class={s.actions}>
							<Show when={update.mrUrl}>
								<a class={s.link} href={update.mrUrl} target="_blank" rel="noreferrer">
									MR
								</a>
							</Show>
							<Button
								type="button"
								variant="primary"
								disabled={props.busy}
								onClick={() => props.onPull(update)}
							>
								{m.chatUpdatePull()}
							</Button>
						</div>
					</article>
				)}
			</For>
		</Show>
	);
}

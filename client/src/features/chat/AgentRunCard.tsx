import { Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { codingStatus } from "@/features/coding/CodingTaskDetail";
import type { components } from "@/shared/api/schema";
import { AgentWorkTabs } from "./AgentWorkTabs";
import s from "./ChatPage.module.css";

type CodingTaskDetails = components["schemas"]["CodingTaskDetails"];

export function AgentRunCard(props: { task: CodingTaskDetails }) {
	return (
		<article class={s.agentPanel}>
			<header class={s.agentHeader}>
				<div>
					<strong>
						{m.chatAgentTask()} #{props.task.id}
					</strong>
					<p>{codingStatus(props.task.phase)}</p>
				</div>
				<Show when={props.task.mergeRequestUrl}>
					{(url) => (
						<a href={url()} target="_blank" rel="noopener noreferrer">
							{m.openMergeRequest()} !{props.task.mergeRequestIid}
						</a>
					)}
				</Show>
			</header>
			<p class={s.agentRequest}>{props.task.request}</p>
			<AgentWorkTabs task={props.task} />
		</article>
	);
}

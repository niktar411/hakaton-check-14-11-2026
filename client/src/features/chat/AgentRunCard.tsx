import { For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { codingStatus } from "@/features/coding/CodingTaskDetail";
import type { components } from "@/shared/api/schema";
import s from "./ChatPage.module.css";

type CodingTaskDetails = components["schemas"]["CodingTaskDetails"];
type CodingMessage = components["schemas"]["CodingMessage"];

function toolSummary(message: CodingMessage) {
	try {
		const data = JSON.parse(message.content) as {
			input?: { path?: string; check?: string };
		};
		return data.input?.path || data.input?.check || "";
	} catch {
		return "";
	}
}

export function AgentRunCard(props: { task: CodingTaskDetails }) {
	return (
		<article class={s.bubble} data-role="assistant">
			<strong>
				{m.chatAgentTask()} #{props.task.id}
			</strong>
			<p>{codingStatus(props.task.phase)}</p>
			<p class={s.agentRequest}>{props.task.request}</p>
			<Show when={props.task.mergeRequestUrl}>
				{(url) => (
					<a href={url()} target="_blank" rel="noopener noreferrer">
						{m.openMergeRequest()} !{props.task.mergeRequestIid}
					</a>
				)}
			</Show>
			<ul class={s.agentActivity}>
				<For
					each={(props.task.messages ?? [])
						.filter((message) => message.role === "tool")
						.slice(-5)}
				>
					{(message) => (
						<li>
							<code>{message.toolName}</code> {toolSummary(message)}
						</li>
					)}
				</For>
			</ul>
			<Show when={props.task.phase === "running"}>
				<p class={s.agentHint}>{m.chatAgentRunning()}</p>
			</Show>
		</article>
	);
}

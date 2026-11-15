import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import { codingStatus } from "@/features/coding/CodingTaskDetail";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import s from "./AdministrationPage.module.css";

export function CodingTasksAdminPanel() {
	const [offset, setOffset] = createSignal(0);
	const [selectedId, setSelectedId] = createSignal<number>();
	const [tasks, { refetch }] = createResource(offset, async (value) =>
		unwrap(await api.GET("/api/admin/coding/tasks", { params: { query: { offset: value } } })),
	);
	const [task, { refetch: refetchTask }] = createResource(
		() => selectedId() ?? null,
		async (id) => unwrap(await api.GET("/api/admin/coding/tasks/{id}", { params: { path: { id } } })),
	);

	return (
		<section class={s.panel}>
			<div class={s.toolbar}>
				<div><h2>{m.codingTasks()}</h2><p class={s.muted}>{m.adminCodingViewerHint()}</p></div>
				<Button onClick={() => void refetch()}>{m.retry()}</Button>
			</div>
			<Show when={!tasks.loading} fallback={<LoadingState />}>
				<Show when={!tasks.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<div class={s.workspace}>
						<div class={s.list}>
							<For each={tasks() ?? []} fallback={<EmptyState message={m.codingTasksEmpty()} />}>
								{(item) => (
									<button type="button" class={s.choiceButton} data-active={selectedId() === item.id} aria-pressed={selectedId() === item.id} onClick={() => setSelectedId(item.id)}>
										<strong>#{item.id} · {item.request}</strong><br />
										<span>{codingStatus(item.phase)}</span><br />
										<time>{formatDate(item.updatedAt)}</time>
									</button>
								)}
							</For>
							<div class={s.actions}>
								<Button disabled={offset() === 0} onClick={() => { setOffset((value) => Math.max(0, value - 100)); setSelectedId(undefined); }}>{m.previous()}</Button>
								<Button disabled={(tasks()?.length ?? 0) < 100} onClick={() => { setOffset((value) => value + 100); setSelectedId(undefined); }}>{m.next()}</Button>
							</div>
						</div>
						<div class={s.viewer}>
							<Show when={selectedId()} fallback={<EmptyState message={m.selectCodingTask()} />}>
								<Show when={!task.loading} fallback={<LoadingState />}>
									<Show when={!task.error} fallback={<ErrorState onRetry={() => void refetchTask()} />}>
										<Show when={task()}>
											{(data) => (
												<>
													<div><h2>{m.codingTask()} #{data().id}</h2><p>{codingStatus(data().phase)} · {data().accountId}</p></div>
													<p class={s.message}>{data().request}</p>
													<Show when={data().clarification}><section><h3>{m.currentClarification()}</h3><p class={s.message}>{data().clarification}</p></section></Show>
													<Show when={data().failureReason}><p role="alert">{data().failureReason}</p></Show>
													<Show when={data().mergeRequestIid}>
														{(iid) => <section><h3>{m.mergeRequestStatus()}</h3><Show when={data().mergeRequestUrl} fallback={<p>!{iid()}</p>}>{(url) => <a href={url()} target="_blank" rel="noopener noreferrer">!{iid()}</a>}</Show><p>{data().branch}</p></section>}
													</Show>
													<section class={s.messages}><h3>{m.codingMessages()}</h3>
														<For each={data().messages ?? []} fallback={<EmptyState message={m.codingMessagesEmpty()} />}>
															{(message) => (
																<article class={s.message}>
																	<strong>{message.role === "tool" ? `${m.toolActivity()}: ${message.toolName ?? ""}` : message.role === "user" ? m.you() : message.role === "assistant" ? m.assistant() : m.systemMessage()}</strong>
																	<Show when={message.toolName === "run_check"}><p>{m.checkStatus()}</p></Show>
																	<p>{message.content}</p><time>{formatDate(message.createdAt)}</time>
																</article>
															)}
														</For>
													</section>
												</>
											)}
										</Show>
									</Show>
								</Show>
							</Show>
						</div>
					</div>
				</Show>
			</Show>
		</section>
	);
}

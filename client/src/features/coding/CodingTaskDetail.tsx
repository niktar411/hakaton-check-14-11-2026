import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, type ApiResult, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./CodingPage.module.css";

type CodingTask = components["schemas"]["CodingTask"];
type CodingMessage = components["schemas"]["CodingMessage"];

export function codingStatus(phase: string) {
	switch (phase) {
		case "clarifying":
			return m.codingStatusClarifying();
		case "awaiting_confirmation":
			return m.codingStatusAwaitingConfirmation();
		case "running":
			return m.codingStatusRunning();
		case "awaiting_merge":
			return m.codingStatusAwaitingMerge();
		case "failed":
			return m.codingStatusFailed();
		default:
			return phase;
	}
}

export default function CodingTaskDetail(props: {
	id: number;
	csrf?: string;
	onChange: (task: CodingTask) => void;
	onRework: (task: CodingTask) => void;
}) {
	const toast = useToast();
	const [task, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/coding/tasks/{id}", { params: { path: { id: props.id } } })),
	);
	const [answer, setAnswer] = createSignal("");
	const [confirmation, setConfirmation] = createSignal("");
	const [reworkRequest, setReworkRequest] = createSignal("");
	const [showRework, setShowRework] = createSignal(false);
	const [pending, setPending] = createSignal("");

	async function run(name: string, operation: Promise<ApiResult<CodingTask>>) {
		if (pending()) return undefined;
		setPending(name);
		try {
			const updated = unwrap(await operation);
			props.onChange(updated);
			if (updated.id === props.id) await refetch();
			toast.show(m.success());
			return updated;
		} catch {
			toast.show(m.loadError());
			return undefined;
		} finally {
			setPending("");
		}
	}

	async function submitAnswer(event: SubmitEvent) {
		event.preventDefault();
		const value = answer().trim();
		if (!value) return;
		const updated = await run("answer", api.POST("/api/coding/tasks/{id}/answer", {
			headers: csrfHeaders(props.csrf),
			params: { path: { id: props.id } },
			body: { answer: value },
		}));
		if (updated) setAnswer("");
	}

	async function submitConfirmation(event: SubmitEvent) {
		event.preventDefault();
		const value = confirmation().trim();
		if (!value || task()?.phase !== "awaiting_confirmation") return;
		const updated = await run("confirm", api.POST("/api/coding/tasks/{id}/confirm", {
			headers: csrfHeaders(props.csrf),
			params: { path: { id: props.id } },
			body: { confirmation: value, understood: value },
		}));
		if (updated) setConfirmation("");
	}

	async function submitRework(event: SubmitEvent) {
		event.preventDefault();
		const value = reworkRequest().trim();
		if (!value) return;
		setAnswer("");
		setConfirmation("");
		const updated = await run("rework", api.POST("/api/coding/tasks/{id}/rework", {
			headers: csrfHeaders(props.csrf),
			params: { path: { id: props.id } },
			body: { request: value },
		}));
		if (updated) {
			props.onRework(updated);
			setReworkRequest("");
			setShowRework(false);
		}
	}

	const regularMessages = () => (task()?.messages ?? []).filter((message) => message.role !== "tool");
	const toolMessages = () => (task()?.messages ?? []).filter((message) => message.role === "tool");
	const messageRole = (message: CodingMessage) => message.role === "user" ? m.you() : message.role === "assistant" ? m.assistant() : m.systemMessage();

	return (
		<section class={s.detail} aria-live="polite">
			<Show when={!task.loading} fallback={<LoadingState />}>
				<Show when={!task.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<Show when={task()}>
						{(data) => (
							<>
								<header class={s.detailHeader}>
									<div><h2>{m.codingTask()} #{data().id}</h2><p>{codingStatus(data().phase)}</p></div>
									<Show when={data().parentId}><span>{m.reworkOf()} #{data().parentId}</span></Show>
								</header>
								<p class={s.request}>{data().request}</p>
								<Show when={data().failureReason}><p role="alert">{data().failureReason}</p></Show>
								<Show when={data().clarification}>
									<section class={s.subsection}><h3>{m.currentClarification()}</h3><p class={s.request}>{data().clarification}</p></section>
								</Show>
								<p class={s.freshHint}>{m.codingFreshClarificationHint()}</p>

								<section class={s.subsection}>
									<h3>{m.codingMessages()}</h3>
									<For each={regularMessages()} fallback={<EmptyState message={m.codingMessagesEmpty()} />}>
										{(message) => (
											<article class={s.message}>
												<strong>{messageRole(message)}</strong>
												<p>{message.content}</p>
												<time>{formatDate(message.createdAt)}</time>
											</article>
										)}
									</For>
								</section>

								<Show when={data().phase === "clarifying" || data().phase === "awaiting_confirmation"}>
									<form class={s.actionForm} onSubmit={submitAnswer}>
										<label for={`answer-${data().id}`}>{m.clarificationLabel()}</label>
										<textarea id={`answer-${data().id}`} required rows={4} maxlength={16000} value={answer()} onInput={(event) => setAnswer(event.currentTarget.value)} />
										<Button type="submit" disabled={!!pending() || !answer().trim()}>
											{pending() === "answer" ? m.loading() : m.sendClarification()}
										</Button>
									</form>
								</Show>

								<Show when={data().phase === "awaiting_confirmation"}>
									<form class={s.actionForm} onSubmit={submitConfirmation}>
										<p>{m.confirmationHint()}</p>
										<label for={`confirmation-${data().id}`}>{m.confirmationLabel()}</label>
										<textarea id={`confirmation-${data().id}`} required rows={2} maxlength={200} value={confirmation()} onInput={(event) => setConfirmation(event.currentTarget.value)} />
										<Button type="submit" variant="primary" disabled={!!pending() || !confirmation().trim()}>
											{pending() === "confirm" ? m.loading() : m.confirmTask()}
										</Button>
									</form>
								</Show>

								<section class={s.subsection}>
									<h3>{m.toolActivity()}</h3>
									<For each={toolMessages()} fallback={<EmptyState message={m.toolActivityEmpty()} />}>
										{(activity) => (
											<article class={s.activity}>
												<strong>{activity.toolName || m.toolActivity()}</strong>
												<Show when={activity.toolName === "run_check"}><span>{m.checkStatus()}</span></Show>
												<p>{activity.content}</p>
												<time>{formatDate(activity.createdAt)}</time>
											</article>
										)}
									</For>
								</section>

								<section class={s.subsection}>
									<h3>{m.mergeRequestStatus()}</h3>
									<Show when={data().mergeRequestIid} fallback={<EmptyState message={m.mergeRequestEmpty()} />}>
										{(iid) => (
											<div>
												<Show when={data().mergeRequestUrl} fallback={<span>!{iid()}</span>}>
													{(url) => <a href={url()} target="_blank" rel="noopener noreferrer">!{iid()}</a>}
												</Show>
												<p>{codingStatus(data().phase)}</p>
												<Show when={data().branch}><p>{data().branch}</p></Show>
											</div>
										)}
									</Show>
								</section>

								<div class={s.rework}>
									<Button onClick={() => setShowRework((value) => !value)}>{m.requestRework()}</Button>
									<Show when={showRework()}>
										<form class={s.actionForm} onSubmit={submitRework}>
											<p>{m.reworkHint()}</p>
											<label for={`rework-${data().id}`}>{m.reworkLabel()}</label>
											<textarea id={`rework-${data().id}`} required rows={4} maxlength={16000} value={reworkRequest()} onInput={(event) => setReworkRequest(event.currentTarget.value)} />
											<Button type="submit" disabled={!!pending() || !reworkRequest().trim()}>
												{pending() === "rework" ? m.loading() : m.sendRework()}
											</Button>
										</form>
									</Show>
								</div>
							</>
						)}
					</Show>
				</Show>
			</Show>
		</section>
	);
}

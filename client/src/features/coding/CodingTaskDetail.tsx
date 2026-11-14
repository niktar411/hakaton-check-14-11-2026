import { createEffect, createResource, createSignal, For, onCleanup, Show } from "solid-js";
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

type DetailTab = "dev" | "read" | "changes" | "checks";
type ToolAudit = {
	operation?: string;
	input?: {
		path?: string;
		query?: string;
		check?: string;
		content?: string;
		plannerBlockId?: number;
		title?: string;
	};
	output?: { ok?: boolean; content?: string; error?: string };
};

function parseToolAudit(message: CodingMessage): ToolAudit {
	try {
		return JSON.parse(message.content) as ToolAudit;
	} catch {
		return { operation: message.toolName };
	}
}

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
	const [tab, setTab] = createSignal<DetailTab>("dev");

	createEffect(() => {
		if (task()?.phase !== "running") return;
		const timer = window.setInterval(() => void refetch(), 4000);
		onCleanup(() => window.clearInterval(timer));
	});

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
	const readingMessages = () =>
		toolMessages().filter((message) =>
			["list_files", "read_file", "search_code"].includes(message.toolName ?? ""),
		);
	const changeMessages = () =>
		toolMessages().filter((message) =>
			["write_file", "create_merge_request", "start_planner_block", "workspace"].includes(message.toolName ?? ""),
		);
	const checkMessages = () => toolMessages().filter((message) => message.toolName === "run_check");
	const messageRole = (message: CodingMessage) =>
		message.role === "user" ? m.you() : message.role === "assistant" ? m.assistant() : m.systemMessage();
	const toolTarget = (audit: ToolAudit) =>
		audit.input?.path || audit.input?.query || audit.input?.check || (audit.input?.plannerBlockId ? `#${audit.input.plannerBlockId}` : "");

	return (
		<section class={s.detail}>
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

								<nav class={s.detailTabs} aria-label={m.codingTabs()}>
									<button type="button" data-active={tab() === "dev"} aria-pressed={tab() === "dev"} onClick={() => setTab("dev")}>
										{m.codingTabDev()} <span>{regularMessages().length}</span>
									</button>
									<button type="button" data-active={tab() === "read"} aria-pressed={tab() === "read"} onClick={() => setTab("read")}>
										{m.codingTabRead()} <span>{readingMessages().length}</span>
									</button>
									<button type="button" data-active={tab() === "changes"} aria-pressed={tab() === "changes"} onClick={() => setTab("changes")}>
										{m.codingTabChanges()} <span>{changeMessages().length}</span>
									</button>
									<button type="button" data-active={tab() === "checks"} aria-pressed={tab() === "checks"} onClick={() => setTab("checks")}>
										{m.codingTabChecks()} <span>{checkMessages().length}</span>
									</button>
								</nav>

								<Show when={tab() === "dev"}>
									<section class={s.tabPanel} aria-live="polite">
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
								</Show>

								<Show when={tab() === "read"}>
									<section class={s.tabPanel}>
										<For each={readingMessages()} fallback={<EmptyState message={m.codingReadEmpty()} />}>
											{(message) => {
												const audit = parseToolAudit(message);
												return (
													<article class={s.fileCard}>
														<header class={s.fileHeader}>
															<strong>{audit.operation || message.toolName}</strong>
															<Show when={toolTarget(audit)}><span class={s.filePath}>{toolTarget(audit)}</span></Show>
														</header>
														<pre class={s.fileContent}>{audit.output?.content || message.content}</pre>
														<time>{formatDate(message.createdAt)}</time>
													</article>
												);
											}}
										</For>
									</section>
								</Show>

								<Show when={tab() === "changes"}>
									<section class={s.tabPanel}>
										<For each={changeMessages()} fallback={<EmptyState message={m.codingChangesEmpty()} />}>
											{(message) => {
												const audit = parseToolAudit(message);
												return (
													<article class={s.fileCard}>
														<header class={s.fileHeader}>
															<strong>{audit.operation || message.toolName}</strong>
															<Show when={toolTarget(audit)}><span class={s.filePath}>{toolTarget(audit)}</span></Show>
														</header>
														<Show when={audit.input?.content}><pre class={s.fileContent}>{audit.input?.content}</pre></Show>
														<Show when={audit.output?.content}><pre class={s.fileContent}>{audit.output?.content}</pre></Show>
														<Show when={audit.output?.ok === false}><p class={s.checkFailed} role="alert">{audit.output?.error || m.codingCheckFailed()}</p></Show>
														<time>{formatDate(message.createdAt)}</time>
													</article>
												);
											}}
										</For>
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
									</section>
								</Show>

								<Show when={tab() === "checks"}>
									<section class={s.tabPanel}>
										<For each={checkMessages()} fallback={<EmptyState message={m.codingChecksEmpty()} />}>
											{(message) => {
												const audit = parseToolAudit(message);
												return (
													<article class={s.fileCard}>
														<header class={s.fileHeader}>
															<strong>{audit.input?.check || message.toolName}</strong>
														</header>
														<p class={audit.output?.ok ? s.checkOk : s.checkFailed} role="status">
															{audit.output?.ok ? m.codingCheckOk() : m.codingCheckFailed()}
														</p>
														<pre class={s.fileContent}>{audit.output?.content || audit.output?.error || message.content}</pre>
														<time>{formatDate(message.createdAt)}</time>
													</article>
												);
											}}
										</For>
									</section>
								</Show>

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

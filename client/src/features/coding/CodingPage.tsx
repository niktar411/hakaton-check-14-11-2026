import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, isAuthError, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";

type CodingTask = components["schemas"]["CodingTask"];
type LoginResult = components["schemas"]["LoginResult"];
import { LocalLoginForm } from "@/features/auth/LocalLoginForm";
import CodingTaskDetail, {
	codingStatus,
} from "@/features/coding/CodingTaskDetail";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./CodingPage.module.css";

type Session = {
	csrf: string;
	username: string;
};

export default function CodingPage() {
	const toast = useToast();
	const [session, setSession] = createSignal<Session>();
	const [authRequired, setAuthRequired] = createSignal(false);
	const [loadFailed, setLoadFailed] = createSignal(false);
	const [selectedId, setSelectedId] = createSignal<number>();
	const [request, setRequest] = createSignal("");
	const [creating, setCreating] = createSignal(false);
	const [tasks, { refetch }] = createResource(async () => {
		try {
			const result = unwrap(await api.GET("/api/coding/tasks"));
			setAuthRequired(false);
			setLoadFailed(false);
			if (!session()) {
				try {
					const current = unwrap(await api.GET("/api/me"));
					setSession({ csrf: current.csrf, username: current.username });
				} catch {
					setAuthRequired(true);
					return [];
				}
			}
			return result;
		} catch (error) {
			if (isAuthError(error)) {
				setAuthRequired(true);
				setLoadFailed(false);
			} else {
				setAuthRequired(false);
				setLoadFailed(true);
			}
			return [];
		}
	});

	createEffect(() => {
		const rows = tasks();
		if (!rows?.length) {
			setSelectedId(undefined);
			return;
		}
		if (!rows.some((item) => item.id === selectedId())) {
			setSelectedId(rows[0]?.id);
		}
	});

	async function createTask(event: SubmitEvent) {
		event.preventDefault();
		const value = request().trim();
		if (!value || creating()) return;
		setCreating(true);
		try {
			const task = unwrap(
				await api.POST("/api/coding/tasks", {
					headers: csrfHeaders(session()?.csrf),
					body: { request: value },
				}),
			);
			setRequest("");
			await refetch();
			setSelectedId(task.id);
			toast.show(m.codingTaskCreated());
		} catch {
			toast.show(m.loadError());
		} finally {
			setCreating(false);
		}
	}

	function loggedIn(value: LoginResult) {
		setSession({ csrf: value.csrf, username: value.account.username });
		setAuthRequired(false);
		setLoadFailed(false);
		void refetch();
	}

	return (
		<main class={s.page}>
			<header class={s.header}>
				<h1>{m.codingPageTitle()}</h1>
				<p>{m.codingIntro()}</p>
			</header>
			<Show when={!tasks.loading} fallback={<LoadingState />}>
				<Show
					when={!authRequired()}
					fallback={
						<section class={s.login}>
							<h2>{m.codingLoginTitle()}</h2>
							<p>{m.codingLoginHint()}</p>
							<LocalLoginForm audience="coding" onSuccess={loggedIn} />
						</section>
					}
				>
					<Show
						when={!loadFailed()}
						fallback={
							<ErrorState
								onRetry={() => {
									void refetch();
								}}
							/>
						}
					>
						<section class={s.account} aria-label={m.localAccountArea()}>
							<strong>{m.localAccountArea()}</strong>
							<span>{session()?.username ?? m.localSessionActive()}</span>
						</section>
						<form class={s.createForm} onSubmit={createTask}>
							<label for="new-coding-request">{m.newTaskLabel()}</label>
							<textarea
								id="new-coding-request"
								required
								rows={4}
								maxlength={16000}
								value={request()}
								onInput={(event) => setRequest(event.currentTarget.value)}
								placeholder={m.newTaskPlaceholder()}
							/>
							<p>{m.codingFreshClarificationHint()}</p>
							<Button
								type="submit"
								variant="primary"
								disabled={creating() || !request().trim()}
							>
								{creating() ? m.loading() : m.createTask()}
							</Button>
						</form>
						<div class={s.workspace}>
							<aside class={s.taskList} aria-label={m.codingTasks()}>
								<div class={s.listHeader}>
									<h2>{m.codingTasks()}</h2>
									<Button disabled={tasks.loading} onClick={() => void refetch()}>
										{m.retry()}
									</Button>
								</div>
								<For
									each={tasks() ?? []}
									fallback={<EmptyState message={m.codingTasksEmpty()} />}
								>
									{(task) => (
										<button
											type="button"
											class={s.taskButton}
											data-selected={selectedId() === task.id}
											aria-pressed={selectedId() === task.id}
											onClick={() => setSelectedId(task.id)}
										>
											<strong>
												#{task.id} {task.request}
											</strong>
											<span>{codingStatus(task.phase)}</span>
											<time>{formatDate(task.updatedAt)}</time>
										</button>
									)}
								</For>
							</aside>
							<Show when={selectedId()} keyed>
								{(id) => (
									<CodingTaskDetail
										id={id}
										csrf={session()?.csrf}
										onChange={() => {
											void refetch();
										}}
										onRework={(task: CodingTask) => {
											void Promise.resolve(refetch()).then(() =>
												setSelectedId(task.id),
											);
										}}
									/>
								)}
							</Show>
						</div>
					</Show>
				</Show>
			</Show>
		</main>
	);
}

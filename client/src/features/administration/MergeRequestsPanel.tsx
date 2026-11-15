import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { codingStatus } from "@/features/coding/CodingTaskDetail";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type CodingTask = components["schemas"]["CodingTask"];
type MergeResult = components["schemas"]["MergeResult"];

export function MergeRequestsPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [tasks, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/coding/tasks", { params: { query: { offset: 0 } } })),
	);
	const [selected, setSelected] = createSignal<CodingTask>();
	const [sha, setSha] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [result, setResult] = createSignal<MergeResult>();
	const mergeRequests = () => (tasks() ?? []).filter((task) => task.mergeRequestIid !== undefined);

	async function merge() {
		const task = selected();
		const iid = task?.mergeRequestIid;
		if (!task || !iid || !sha().trim() || pending()) return;
		setPending(true);
		try {
			const merged = unwrap(await api.POST("/api/admin/merge-requests/{iid}/merge", {
				headers: csrfHeaders(props.csrf), params: { path: { iid } }, body: { sha: sha().trim() },
			}));
			setResult(merged); setSelected(undefined); setSha("");
			await refetch(); toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div class={s.toolbar}><div><h2>{m.adminMergeRequests()}</h2><p class={s.muted}>{m.mergeAdminHint()}</p></div><Button onClick={() => void refetch()}>{m.retry()}</Button></div>
			<Show when={result()}>
				{(value) => <p role="status">!{value().iid} · {value().state} · {value().sha}</p>}
			</Show>
			<Show when={!tasks.loading} fallback={<LoadingState />}>
				<Show when={!tasks.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<div class={s.list}>
						<For each={mergeRequests()} fallback={<EmptyState message={m.mergeRequestsEmpty()} />}>
							{(task) => (
								<article class={s.card}>
									<div><h3>!{task.mergeRequestIid} · {m.codingTask()} #{task.id}</h3><p>{task.request}</p></div>
									<p>{codingStatus(task.phase)}</p><p>{task.branch}</p>
									<div class={s.actions}>
										<Show when={task.mergeRequestUrl}>{(url) => <a href={url()} target="_blank" rel="noopener noreferrer">{m.openMergeRequest()}</a>}</Show>
										<Button variant="primary" onClick={() => { setSha(""); setSelected(task); }}>{m.merge()}</Button>
									</div>
								</article>
							)}
						</For>
					</div>
				</Show>
			</Show>
			<ConfirmDialog
				open={!!selected()} title={m.confirmMerge()} description={`!${selected()?.mergeRequestIid ?? ""} · ${selected()?.branch ?? ""}`}
				confirmLabel={m.merge()} confirmDisabled={!sha().trim()} pending={pending()} danger
				onClose={() => { if (!pending()) setSelected(undefined); }} onConfirm={() => void merge()}
			>
				<p>{m.mergeConfirmationHint()}</p>
				<Input label={m.mergeSha()} required maxlength={128} value={sha()} onInput={(event) => setSha(event.currentTarget.value)} />
			</ConfirmDialog>
		</section>
	);
}

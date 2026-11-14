import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

const historyLimit = 50;

export function FaqPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [context, setContext] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [offset, setOffset] = createSignal(0);
	const [current, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/questions/context")),
	);
	const [history, { refetch: refetchHistory }] = createResource(offset, async (value) =>
		unwrap(await api.GET("/api/admin/questions/history", { params: { query: { limit: historyLimit, offset: value } } })),
	);
	createEffect(() => {
		const data = current();
		if (data) setContext(data.context);
	});

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (pending()) return;
		setPending(true);
		try {
			const result = unwrap(await api.PUT("/api/admin/questions/context", {
				headers: csrfHeaders(props.csrf), body: { context: context() },
			}));
			setContext(result.context);
			toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div><h2>{m.adminFaq()}</h2><p class={s.muted}>{m.faqContextHint()}</p></div>
			<Show when={!current.loading} fallback={<LoadingState />}>
				<Show when={!current.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<form class={s.form} onSubmit={save}>
						<label for="faq-context">{m.faqContext()}</label>
						<textarea id="faq-context" rows={14} required value={context()} onInput={(event) => setContext(event.currentTarget.value)} />
						<Button type="submit" variant="primary" disabled={pending() || !context().trim()}>{pending() ? m.loading() : m.save()}</Button>
					</form>
				</Show>
			</Show>
			<section class={s.section}>
				<div class={s.toolbar}><h2>{m.faqHistory()}</h2><Button onClick={() => void refetchHistory()}>{m.retry()}</Button></div>
				<Show when={!history.loading} fallback={<LoadingState />}>
					<Show when={!history.error} fallback={<ErrorState onRetry={() => void refetchHistory()} />}>
						<For each={history()?.items ?? []} fallback={<EmptyState message={m.faqHistoryEmpty()} />}>
							{(entry) => (
								<article class={s.card}>
									<time>{formatDate(entry.createdAt)}</time>
									<h3>{entry.question}</h3>
									<p class={s.message}>{entry.answer}</p>
									<Show when={(entry.links?.length ?? 0) > 0}>
										<ul><For each={entry.links ?? []}>{(link) => <li><a href={link.path} target="_blank" rel="noopener noreferrer">{link.title}</a></li>}</For></ul>
									</Show>
								</article>
							)}
						</For>
						<div class={s.actions}>
							<Button disabled={offset() === 0} onClick={() => setOffset((value) => Math.max(0, value - historyLimit))}>{m.previous()}</Button>
							<Button disabled={(history()?.items?.length ?? 0) < historyLimit} onClick={() => setOffset((value) => value + historyLimit)}>{m.next()}</Button>
						</div>
					</Show>
				</Show>
			</section>
		</section>
	);
}

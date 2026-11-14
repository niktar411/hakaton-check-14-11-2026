import { createEffect, createResource, createSignal, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import { Button } from "@/shared/ui/Button";
import { ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

export function ChatContextPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [general, setGeneral] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [view, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/chat-context")),
	);
	createEffect(() => {
		const data = view();
		if (data) setGeneral(data.general);
	});
	const limit = () => view()?.limit ?? 16000;

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (pending()) return;
		setPending(true);
		try {
			const result = unwrap(
				await api.PUT("/api/admin/chat-context", {
					headers: csrfHeaders(props.csrf),
					body: { general: general().trim() },
				}),
			);
			setGeneral(result.general);
			await refetch();
			toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div>
				<h2>{m.adminAiContext()}</h2>
				<p class={s.muted}>{m.aiContextHint()}</p>
			</div>
			<Show when={!view.loading} fallback={<LoadingState />}>
				<Show when={!view.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<section class={s.section}>
						<div>
							<h2>{m.aiContextGeneral()}</h2>
							<p class={s.muted}>{m.aiContextGeneralHint()}</p>
						</div>
						<form class={s.form} onSubmit={save}>
							<textarea
								rows={10}
								maxlength={limit()}
								aria-label={m.aiContextGeneral()}
								value={general()}
								onInput={(event) => setGeneral(event.currentTarget.value)}
							/>
							<p class={s.muted}>
								{m.contextCharacters()}: {general().length} / {limit()}
							</p>
							<p class={s.muted}>{m.aiContextWarning()}</p>
							<Button type="submit" variant="primary" disabled={pending()}>
								{pending() ? m.loading() : m.save()}
							</Button>
						</form>
					</section>
					<section class={s.section}>
						<h2>{m.aiContextRules()}</h2>
						<p class={s.muted}>{m.aiContextRulesText()}</p>
						<p class={s.muted}>
							{m.aiContextSkipPhrase({ phrase: view()?.skipPhrase ?? "" })}
						</p>
					</section>
					<section class={s.section}>
						<h2>{m.aiContextPromptClarify()}</h2>
						<pre class={s.prompt}>{view()?.clarifyingPrompt}</pre>
					</section>
					<section class={s.section}>
						<h2>{m.aiContextPromptSolution()}</h2>
						<pre class={s.prompt}>{view()?.solutionPrompt}</pre>
					</section>
					<section class={s.section}>
						<h2>{m.aiContextAgents()}</h2>
						<p class={s.muted}>{m.aiContextAgentsHint()}</p>
						<pre class={s.prompt}>{view()?.agents}</pre>
					</section>
				</Show>
			</Show>
		</section>
	);
}

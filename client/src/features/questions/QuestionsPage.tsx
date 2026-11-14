import { createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, requestId, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";

type QuestionResult = components["schemas"]["Answer"];
import { Button } from "@/shared/ui/Button";
import { ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./QuestionsPage.module.css";

export default function QuestionsPage() {
	const toast = useToast();
	const [question, setQuestion] = createSignal("");
	const [result, setResult] = createSignal<QuestionResult>();
	const [pending, setPending] = createSignal(false);
	const [failed, setFailed] = createSignal(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		const value = question().trim();
		if (!value || pending()) return;
		setPending(true);
		setFailed(false);
		setResult(undefined);
		try {
			const response = unwrap(
				await api.POST("/api/public/questions", {
					body: { question: value, requestId: requestId() },
				}),
			);
			setResult(response);
		} catch {
			setResult(undefined);
			setFailed(true);
			toast.show(m.questionFailed());
		} finally {
			setPending(false);
		}
	}

	return (
		<main class={s.page}>
			<header class={s.header}>
				<h1>{m.questionsPageTitle()}</h1>
				<p>{m.questionsIntro()}</p>
			</header>
			<form class={s.form} onSubmit={submit}>
				<label for="public-question">{m.questionLabel()}</label>
				<textarea
					id="public-question"
					required
					maxlength={4000}
					rows={5}
					value={question()}
					onInput={(event) => setQuestion(event.currentTarget.value)}
					placeholder={m.questionPlaceholder()}
				/>
				<Button
					type="submit"
					variant="primary"
					disabled={pending() || !question().trim()}
				>
					{pending() ? m.loading() : m.askQuestion()}
				</Button>
			</form>
			<Show when={pending()}>
				<LoadingState label={m.questionLoading()} />
			</Show>
			<Show when={failed() && !pending()}>
				<ErrorState />
			</Show>
			<Show when={result()}>
				{(data) => (
					<section class={s.answer} aria-live="polite">
						<h2>{m.answerTitle()}</h2>
						<p class={s.answerText}>{data().answer}</p>
						<Show when={(data().links?.length ?? 0) > 0}>
							<div class={s.sources}>
								<h3>{m.verifiedLinks()}</h3>
								<ul>
									<For each={data().links ?? []}>
										{(link) => (
											<li>
												<a
													href={link.path}
													target="_blank"
													rel="noopener noreferrer"
												>
													{link.title}
												</a>
											</li>
										)}
									</For>
								</ul>
							</div>
						</Show>
					</section>
				)}
			</Show>
			<Show when={!result() && !pending() && !failed()}>
				<p class={s.hint}>{m.questionsEmptyHint()}</p>
			</Show>
		</main>
	);
}

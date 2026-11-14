import { createResource, createSignal, For, Show } from "solid-js";
import { action, useSubmission } from "@solidjs/router";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, identity } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/Toast";
export default function ChatPage() {
	const [user] = createResource(identity);
	const [chat, setChat] = createSignal<components["schemas"]["ChatResult"]>();
	const [text, setText] = createSignal("");
	const toast = useToast();
	const send = action(async (confirmed: boolean) => {
		try {
			const r = await api.POST("/api/chat", {
				headers: csrfHeaders(user()?.csrf),
				body: {
					id: chat()?.id ?? 0,
					prompt: text().trim(),
					confirm: confirmed,
					requestId: Array.from(
						crypto.getRandomValues(new Uint8Array(24)),
						(x) => x.toString(16).padStart(2, "0"),
					).join(""),
				},
			});
			if (!r.data) {
				toast.show(
					r.error?.detail === "model.access_denied"
						? m.modelAccessDenied()
						: m.loadError(),
				);
				return;
			}
			setChat(r.data);
			setText("");
		} catch {
			toast.show(m.loadError());
		}
	}, "chat-send");
	const submission = useSubmission(send);
	return (
		<main class="space-y-6">
			<h1 class="text-2xl font-bold">{m.chatPageTitle()}</h1>
			<p>{m.chatHint()}</p>
			<Show when={user() && !user()?.modelReady}>
				<p role="status">{m.modelUnavailable()}</p>
			</Show>
			<Button
				disabled={submission.pending}
				onClick={() => {
					setChat(undefined);
					setText("");
				}}
			>
				{m.newChat()}
			</Button>
			<div class="space-y-4" aria-live="polite">
				<For each={chat()?.messages} fallback={<p>{m.empty()}</p>}>
					{(message) => (
						<article class="border-border bg-surface-2 rounded-sm border p-4">
							<h2 class="mb-2 font-bold">
								{message.role === "user" ? m.you() : m.assistant()}
							</h2>
							<p class="wrap-break-word whitespace-pre-wrap">
								{message.content}
							</p>
						</article>
					)}
				</For>
			</div>
			<Show when={submission.pending}>
				<div aria-busy="true" class="bg-surface-2 animate-pulse rounded-md p-8">
					{m.loading()}
				</div>
			</Show>
			<form action={send.with(false)} method="post" class="space-y-4">
				<label for="chat-input" class="block">
					{m.promptLabel()}
				</label>
				<textarea
					id="chat-input"
					required
					maxlength={16000}
					rows={5}
					class="border-border bg-surface w-full rounded-sm border p-4 text-lg"
					value={text()}
					onInput={(e) => setText(e.currentTarget.value)}
				/>
				<div class="flex flex-wrap gap-4">
					<Button
						type="submit"
						disabled={
							submission.pending || !user()?.modelReady || !text().trim()
						}
					>
						{m.send()}
					</Button>
					<Show when={chat()?.phase === "clarifying"}>
						<Button
							type="submit"
							formaction={send.with(true)}
							disabled={submission.pending || !text().trim()}
						>
							{m.confirmSolution()}
						</Button>
					</Show>
				</div>
			</form>
		</main>
	);
}

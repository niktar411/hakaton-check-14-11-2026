import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import { formatDate } from "@/shared/lib/format";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import s from "./AdministrationPage.module.css";

export function ChatViewerPanel() {
	const [accountId, setAccountId] = createSignal<number>();
	const [conversationId, setConversationId] = createSignal<number>();
	const [accounts, { refetch: refetchAccounts }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/accounts", { params: { query: { limit: 100, offset: 0 } } })),
	);
	const [conversations, { refetch: refetchConversations }] = createResource(
		() => accountId() ?? null,
		async (id) => unwrap(await api.GET("/api/admin/accounts/{id}/conversations", {
			params: { path: { id }, query: { limit: 100, offset: 0 } },
		})),
	);
	const [conversation, { refetch: refetchConversation }] = createResource(
		() => conversationId() ?? null,
		async (id) => unwrap(await api.GET("/api/admin/conversations/{id}", { params: { path: { id } } })),
	);

	return (
		<section class={s.panel}>
			<div><h2>{m.adminChats()}</h2><p class={s.muted}>{m.chatViewerHint()}</p></div>
			<Show when={!accounts.loading} fallback={<LoadingState />}>
				<Show when={!accounts.error} fallback={<ErrorState onRetry={() => void refetchAccounts()} />}>
					<div class={s.workspace}>
						<div class={s.list}>
							<h3>{m.adminAccounts()}</h3>
							<For each={accounts() ?? []} fallback={<EmptyState message={m.accountsEmpty()} />}>
								{(account) => (
									<button type="button" class={s.choiceButton} data-active={accountId() === account.id} aria-pressed={accountId() === account.id} onClick={() => { setAccountId(account.id); setConversationId(undefined); }}>
										{account.displayName} · {account.username}
									</button>
								)}
							</For>
							<Show when={accountId()}>
								<h3>{m.conversations()}</h3>
								<Show when={!conversations.loading} fallback={<LoadingState />}>
									<Show when={!conversations.error} fallback={<ErrorState onRetry={() => void refetchConversations()} />}>
										<For each={conversations() ?? []} fallback={<EmptyState message={m.conversationsEmpty()} />}>
											{(item) => (
												<button type="button" class={s.choiceButton} data-active={conversationId() === item.id} aria-pressed={conversationId() === item.id} onClick={() => setConversationId(item.id)}>
													<strong>#{item.id} · {item.phase}</strong><br />
													<time>{formatDate(item.createdAt)}</time>
												</button>
											)}
										</For>
									</Show>
								</Show>
							</Show>
						</div>
						<div class={s.viewer}>
							<Show when={conversationId()} fallback={<EmptyState message={m.selectConversation()} />}>
								<Show when={!conversation.loading} fallback={<LoadingState />}>
									<Show when={!conversation.error} fallback={<ErrorState onRetry={() => void refetchConversation()} />}>
										<Show when={conversation()}>
											{(data) => (
												<>
													<div><h2>{data().username} · #{data().id}</h2><p>{data().phase} · {formatDate(data().createdAt)}</p></div>
													<div class={s.messages}>
														<For each={data().messages ?? []} fallback={<EmptyState message={m.codingMessagesEmpty()} />}>
															{(message) => (
																<article class={s.message}>
																	<strong>{message.role === "user" ? m.you() : message.role === "assistant" ? m.assistant() : m.systemMessage()}</strong>
																	<p>{message.content}</p>
																	<time>{formatDate(message.createdAt)}</time>
																</article>
															)}
														</For>
													</div>
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

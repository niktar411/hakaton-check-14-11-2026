import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type Account = components["schemas"]["LocalAccount"];

const historyLimit = 50;

export function FaqPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [context, setContext] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [offset, setOffset] = createSignal(0);
	const [selectedAccount, setSelectedAccount] = createSignal<Account>();
	const [accountTone, setAccountTone] = createSignal("");
	const [accountContext, setAccountContext] = createSignal("");
	const [accountPending, setAccountPending] = createSignal(false);
	const [current, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/questions/context")),
	);
	const [accounts, { refetch: refetchAccounts }] = createResource(async () =>
		unwrap(
			await api.GET("/api/admin/accounts", {
				params: { query: { limit: 100, offset: 0 } },
			}),
		),
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

	function pickAccount(account: Account) {
		setSelectedAccount(account);
		setAccountTone(account.tone);
		setAccountContext(account.personalContext);
	}

	async function saveAccount(event: SubmitEvent) {
		event.preventDefault();
		const account = selectedAccount();
		if (!account || accountPending()) return;
		setAccountPending(true);
		try {
			const updated = unwrap(
				await api.POST("/api/admin/accounts/{id}/profile", {
					headers: csrfHeaders(props.csrf),
					params: { path: { id: account.id } },
					body: {
						displayName: account.displayName,
						color: account.color,
						tone: accountTone().trim(),
						personalContext: accountContext().trim(),
					},
				}),
			);
			setSelectedAccount(updated);
			setAccountTone(updated.tone);
			setAccountContext(updated.personalContext);
			await refetchAccounts();
			toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setAccountPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div><h2>{m.adminFaq()}</h2><p class={s.muted}>{m.faqContextHint()}</p></div>
			<section class={s.section}>
				<div><h2>{m.faqContext()}</h2><p class={s.muted}>{m.publicContextSectionHint()}</p></div>
				<Show when={!current.loading} fallback={<LoadingState />}>
					<Show when={!current.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
						<form class={s.form} onSubmit={save}>
							<label for="faq-context">{m.faqContext()}</label>
							<textarea id="faq-context" rows={14} required value={context()} onInput={(event) => setContext(event.currentTarget.value)} />
							<p class={s.muted}>{m.contextCharacters()}: {context().length} / 100000</p>
							<Button type="submit" variant="primary" disabled={pending() || !context().trim()}>{pending() ? m.loading() : m.save()}</Button>
						</form>
					</Show>
				</Show>
			</section>
			<section class={s.section}>
				<div><h2>{m.userChatContexts()}</h2><p class={s.muted}>{m.userChatContextsHint()}</p></div>
				<Show when={!accounts.loading} fallback={<LoadingState />}>
					<Show when={!accounts.error} fallback={<ErrorState onRetry={() => void refetchAccounts()} />}>
						<div class={s.workspace}>
							<div class={s.list}>
								<For each={accounts() ?? []} fallback={<EmptyState message={m.accountsEmpty()} />}>
									{(account) => (
										<button
											type="button"
											class={s.choiceButton}
											data-active={selectedAccount()?.id === account.id}
											aria-pressed={selectedAccount()?.id === account.id}
											onClick={() => pickAccount(account)}
										>
											<strong>{account.displayName}</strong><br />
											<span>{account.username}</span>
										</button>
									)}
								</For>
							</div>
							<div class={s.viewer}>
								<Show when={selectedAccount()} fallback={<EmptyState message={m.selectAccountContext()} />}>
									{(account) => (
										<form class={s.form} onSubmit={saveAccount}>
											<div>
												<h3>{account().displayName} · {account().username}</h3>
												<p class={s.muted}>{m.userChatContextsHint()}</p>
											</div>
											<label for="account-tone">{m.accountTone()}</label>
											<textarea id="account-tone" rows={3} maxlength={2000} value={accountTone()} onInput={(event) => setAccountTone(event.currentTarget.value)} />
											<label for="account-context">{m.personalContext()}</label>
											<textarea id="account-context" rows={10} maxlength={16000} value={accountContext()} onInput={(event) => setAccountContext(event.currentTarget.value)} />
											<p class={s.muted}>{m.contextCharacters()}: {accountContext().length} / 16000</p>
											<Button type="submit" variant="primary" disabled={accountPending()}>{accountPending() ? m.loading() : m.save()}</Button>
										</form>
									)}
								</Show>
							</div>
						</div>
					</Show>
				</Show>
			</section>
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

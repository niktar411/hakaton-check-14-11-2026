import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { Dialog } from "@/shared/ui/Dialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type Account = components["schemas"]["LocalAccount"];
type Role = components["schemas"]["CreateAccountRequest"]["role"];
type AccountAction = { account: Account; kind: "enable" | "reset" };

export function AccountsPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [accounts, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/accounts", { params: { query: { limit: 100, offset: 0 } } })),
	);
	const [pending, setPending] = createSignal(false);
	const [createOpen, setCreateOpen] = createSignal(false);
	const [username, setUsername] = createSignal("");
	const [displayName, setDisplayName] = createSignal("");
	const [password, setPassword] = createSignal("");
	const [color, setColor] = createSignal("");
	const [tone, setTone] = createSignal("");
	const [personalContext, setPersonalContext] = createSignal("");
	const [role, setRole] = createSignal<Role>("member");
	const [profile, setProfile] = createSignal<Account>();
	const [passwordAccount, setPasswordAccount] = createSignal<Account>();
	const [action, setAction] = createSignal<AccountAction>();

	async function mutate(operation: () => Promise<unknown>) {
		if (pending()) return false;
		setPending(true);
		try {
			await operation();
			await refetch();
			toast.show(m.success());
			return true;
		} catch {
			toast.show(m.loadError());
			return false;
		} finally {
			setPending(false);
		}
	}

	async function createAccount(event: SubmitEvent) {
		event.preventDefault();
		const ok = await mutate(async () => {
			unwrap(await api.POST("/api/admin/accounts", {
				headers: csrfHeaders(props.csrf),
				body: {
					username: username().trim(), displayName: displayName().trim(), password: password(),
					role: role(), color: color().trim(), tone: tone().trim(), personalContext: personalContext().trim(),
				},
			}));
		});
		if (ok) {
			setCreateOpen(false);
			setUsername(""); setDisplayName(""); setPassword(""); setColor(""); setTone(""); setPersonalContext(""); setRole("member");
		}
	}

	function editProfile(account: Account) {
		setProfile(account);
		setDisplayName(account.displayName);
		setColor(account.color);
		setTone(account.tone);
		setPersonalContext(account.personalContext);
	}

	async function saveProfile(event: SubmitEvent) {
		event.preventDefault();
		const account = profile();
		if (!account) return;
		const ok = await mutate(async () => {
			unwrap(await api.POST("/api/admin/accounts/{id}/profile", {
				headers: csrfHeaders(props.csrf), params: { path: { id: account.id } },
				body: { displayName: displayName().trim(), color: color().trim(), tone: tone().trim(), personalContext: personalContext().trim() },
			}));
		});
		if (ok) setProfile(undefined);
	}

	async function savePassword(event: SubmitEvent) {
		event.preventDefault();
		const account = passwordAccount();
		if (!account) return;
		const ok = await mutate(async () => {
			unwrap(await api.POST("/api/admin/accounts/{id}/password", {
				headers: csrfHeaders(props.csrf), params: { path: { id: account.id } }, body: { password: password() },
			}));
		});
		if (ok) { setPasswordAccount(undefined); setPassword(""); }
	}

	async function applyAction() {
		const value = action();
		if (!value) return;
		const ok = await mutate(async () => {
			if (value.kind === "enable") {
				unwrap(await api.POST("/api/admin/accounts/{id}/enabled", {
					headers: csrfHeaders(props.csrf), params: { path: { id: value.account.id } }, body: { enabled: !value.account.enabled },
				}));
			} else {
				unwrap(await api.POST("/api/admin/accounts/{id}/sessions/reset", {
					headers: csrfHeaders(props.csrf), params: { path: { id: value.account.id } },
				}));
			}
		});
		if (ok) setAction(undefined);
	}

	return (
		<section class={s.panel}>
			<div class={s.toolbar}>
				<div><h2>{m.adminAccounts()}</h2><p class={s.muted}>{m.accountsHint()}</p></div>
				<Button variant="primary" onClick={() => { setPassword(""); setCreateOpen(true); }}>{m.createAccount()}</Button>
			</div>
			<Show when={!accounts.loading} fallback={<LoadingState />}>
				<Show when={!accounts.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<div class={s.list}>
						<For each={accounts() ?? []} fallback={<EmptyState message={m.accountsEmpty()} />}>
							{(account) => (
								<article class={s.card}>
									<div class={s.cardHeader}>
										<div>
											<h3>{account.displayName}</h3>
											<p>
												{account.username} ·{" "}
												{account.role === "admin"
													? m.roleAdmin()
													: m.roleMember()}
											</p>
										</div>
										<span>{account.enabled ? m.enabled() : m.disabled()}</span>
									</div>
									<p class={s.muted}>{formatDate(account.updatedAt)}</p>
									<div class={s.actions}>
										<Button onClick={() => editProfile(account)}>{m.editProfile()}</Button>
										<Button onClick={() => setAction({ account, kind: "enable" })}>{account.enabled ? m.disable() : m.enable()}</Button>
										<Button onClick={() => { setPassword(""); setPasswordAccount(account); }}>{m.changePassword()}</Button>
										<Button onClick={() => setAction({ account, kind: "reset" })}>{m.resetSessions()}</Button>
									</div>
								</article>
							)}
						</For>
					</div>
				</Show>
			</Show>

			<Dialog open={createOpen()} title={m.createAccount()} onClose={() => { if (!pending()) setCreateOpen(false); }}>
				<form class={s.form} onSubmit={createAccount}>
					<Input label={m.username()} required minlength={3} maxlength={32} value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
					<Input label={m.displayName()} required maxlength={100} value={displayName()} onInput={(e) => setDisplayName(e.currentTarget.value)} />
					<Input label={m.password()} type="password" autocomplete="new-password" required minlength={12} maxlength={128} value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
					<Input label={m.accountColor()} required pattern="#[0-9A-Fa-f]{6}" hint={m.accountColorHint()} value={color()} onInput={(e) => setColor(e.currentTarget.value)} />
					<div><span>{m.accountRole()}</span><div class={s.choices}>
						<Button data-selected={role() === "member"} aria-pressed={role() === "member"} onClick={() => setRole("member")}>{m.roleMember()}</Button>
						<Button data-selected={role() === "admin"} aria-pressed={role() === "admin"} onClick={() => setRole("admin")}>{m.roleAdmin()}</Button>
					</div></div>
					<label for="create-tone">{m.accountTone()}</label>
					<textarea id="create-tone" rows={3} maxlength={2000} value={tone()} onInput={(e) => setTone(e.currentTarget.value)} />
					<label for="create-context">{m.personalContext()}</label>
					<textarea id="create-context" rows={5} maxlength={16000} value={personalContext()} onInput={(e) => setPersonalContext(e.currentTarget.value)} />
					<Button type="submit" variant="primary" disabled={pending() || !username().trim() || !displayName().trim() || !password() || !color().trim()}>{pending() ? m.loading() : m.createAccount()}</Button>
				</form>
			</Dialog>

			<Dialog open={!!profile()} title={m.editProfile()} onClose={() => { if (!pending()) setProfile(undefined); }}>
				<form class={s.form} onSubmit={saveProfile}>
					<Input label={m.displayName()} required maxlength={100} value={displayName()} onInput={(e) => setDisplayName(e.currentTarget.value)} />
					<Input label={m.accountColor()} required pattern="#[0-9A-Fa-f]{6}" hint={m.accountColorHint()} value={color()} onInput={(e) => setColor(e.currentTarget.value)} />
					<label for="profile-tone">{m.accountTone()}</label>
					<textarea id="profile-tone" rows={3} maxlength={2000} value={tone()} onInput={(e) => setTone(e.currentTarget.value)} />
					<label for="profile-context">{m.personalContext()}</label>
					<textarea id="profile-context" rows={6} maxlength={16000} value={personalContext()} onInput={(e) => setPersonalContext(e.currentTarget.value)} />
					<Button type="submit" variant="primary" disabled={pending() || !displayName().trim() || !color().trim()}>{pending() ? m.loading() : m.save()}</Button>
				</form>
			</Dialog>

			<Dialog open={!!passwordAccount()} title={m.changePassword()} onClose={() => { if (!pending()) setPasswordAccount(undefined); }}>
				<form class={s.form} onSubmit={savePassword}>
					<p>{passwordAccount()?.username}</p>
					<Input label={m.newPassword()} type="password" autocomplete="new-password" required minlength={12} maxlength={128} value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
					<Button type="submit" variant="primary" disabled={pending() || password().length < 12}>{pending() ? m.loading() : m.save()}</Button>
				</form>
			</Dialog>

			<ConfirmDialog
				open={!!action()} title={action()?.kind === "reset" ? m.resetSessions() : m.confirmAction()}
				description={action()?.kind === "reset" ? m.resetSessionsHint() : `${action()?.account.username ?? ""} · ${action()?.account.enabled ? m.disable() : m.enable()}`}
				pending={pending()} danger={action()?.kind === "reset" || !!action()?.account.enabled}
				onClose={() => { if (!pending()) setAction(undefined); }} onConfirm={() => void applyAction()}
			/>
		</section>
	);
}

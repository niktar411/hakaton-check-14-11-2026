import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components, paths } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type Action = paths["/api/admin/deployments/{action}"]["post"]["parameters"]["path"]["action"];
type Target = components["schemas"]["RunDeploymentInputBody"]["target"];
const actions: { id: Action; label: () => string }[] = [
	{ id: "deploy", label: () => m.deploymentDeploy() },
	{ id: "rollback", label: () => m.deploymentRollback() },
	{ id: "restart", label: () => m.deploymentRestart() },
	{ id: "start", label: () => m.deploymentStart() },
	{ id: "stop", label: () => m.deploymentStop() },
	{ id: "create_test_database", label: () => m.deploymentCreateTestDatabase() },
	{ id: "drop_test_database", label: () => m.deploymentDropTestDatabase() },
];
const targets: { id: Target; label: () => string }[] = [
	{ id: "production", label: () => m.targetProduction() },
	{ id: "coding", label: () => m.targetCoding() },
	{ id: "review", label: () => m.targetReview() },
	{ id: "test", label: () => m.targetTest() },
];

export function DeploymentsPanel(props: { csrf?: string; member?: boolean }) {
	const toast = useToast();
	const member = () => props.member === true;
	const [offset, setOffset] = createSignal(0);
	const [items, { refetch }] = createResource(offset, async (value) => {
		if (member()) {
			return unwrap(
				await api.GET("/api/deployments", {
					params: { query: { limit: 50, offset: value } },
				}),
			);
		}
		return unwrap(
			await api.GET("/api/admin/deployments", {
				params: { query: { limit: 50, offset: value } },
			}),
		);
	});
	const [action, setAction] = createSignal<Action>();
	const [target, setTarget] = createSignal<Target>(member() ? "coding" : "production");
	const [release, setRelease] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const availableTargets = () =>
		member() ? targets.filter((item) => item.id !== "production") : targets;

	async function run() {
		const current = action();
		if (!current || pending()) return;
		setPending(true);
		try {
			const body = {
				target: target(),
				...(release().trim() ? { release: release().trim() } : {}),
			};
			if (member()) {
				unwrap(
					await api.POST("/api/deployments/{action}", {
						headers: csrfHeaders(props.csrf),
						params: { path: { action: current } },
						body,
					}),
				);
			} else {
				unwrap(
					await api.POST("/api/admin/deployments/{action}", {
						headers: csrfHeaders(props.csrf),
						params: { path: { action: current } },
						body,
					}),
				);
			}
			setAction(undefined); setRelease(""); await refetch(); toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div><h2>{member() ? m.teamDeployments() : m.adminDeployments()}</h2><p class={s.muted}>{member() ? m.teamDeploymentsHint() : m.deploymentsHint()}</p></div>
			<div class={s.choices}>
				<For each={actions}>{(item) => <Button variant={item.id === "stop" || item.id === "drop_test_database" ? "danger" : "secondary"} onClick={() => setAction(item.id)}>{item.label()}</Button>}</For>
			</div>
			<div class={s.toolbar}><h2>{m.deploymentHistory()}</h2><Button onClick={() => void refetch()}>{m.retry()}</Button></div>
			<Show when={!items.loading} fallback={<LoadingState />}>
				<Show when={!items.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<div class={s.list}>
						<For each={items() ?? []} fallback={<EmptyState message={m.deploymentsEmpty()} />}>
							{(item) => (
								<article class={s.card}>
									<div class={s.cardHeader}><h3>#{item.id} · {item.action}</h3><strong>{item.status}</strong></div>
									<p>{item.actor} · {item.target} · {item.release || m.unknown()}</p>
									<Show when={item.backupName}><p>{m.backupName()}: {item.backupName}</p></Show>
									<time>{formatDate(item.createdAt)}</time>
									<Show when={item.finishedAt}><time>{formatDate(item.finishedAt ?? "")}</time></Show>
								</article>
							)}
						</For>
					</div>
					<div class={s.actions}>
						<Button disabled={offset() === 0} onClick={() => setOffset((value) => Math.max(0, value - 50))}>{m.previous()}</Button>
						<Button disabled={(items()?.length ?? 0) < 50} onClick={() => setOffset((value) => value + 50)}>{m.next()}</Button>
					</div>
				</Show>
			</Show>
			<ConfirmDialog
				open={!!action()} title={m.confirmDeployment()} description={`${action() ?? ""} · ${target()}`}
				pending={pending()} danger={action() === "stop" || action() === "drop_test_database" || action() === "rollback"}
				onClose={() => { if (!pending()) setAction(undefined); }} onConfirm={() => void run()}
			>
				<fieldset><legend>{m.deploymentTarget()}</legend><div class={s.choices}>
					<For each={availableTargets()}>{(item) => <Button data-selected={target() === item.id} aria-pressed={target() === item.id} onClick={() => setTarget(item.id)}>{item.label()}</Button>}</For>
				</div></fieldset>
				<Input label={m.deploymentRelease()} maxlength={128} value={release()} onInput={(event) => setRelease(event.currentTarget.value)} hint={m.deploymentReleaseHint()} />
				<p>{m.deploymentConfirmationHint()}</p>
			</ConfirmDialog>
		</section>
	);
}

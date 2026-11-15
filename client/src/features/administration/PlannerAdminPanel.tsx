import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { plannerStatus } from "@/features/planner/PlannerPage";
import { safeColor } from "@/shared/lib/color";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { Dialog } from "@/shared/ui/Dialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type PlannerBlock = components["schemas"]["PlannerBlock"];

export function PlannerAdminPanel(props: { csrf?: string }) {
	const toast = useToast();
	const [blocks, { refetch }] = createResource(async () => unwrap(await api.GET("/api/planner")));
	const [accounts, { refetch: refetchAccounts }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/accounts", { params: { query: { limit: 100, offset: 0 } } })),
	);
	const [formOpen, setFormOpen] = createSignal(false);
	const [editing, setEditing] = createSignal<PlannerBlock>();
	const [deleting, setDeleting] = createSignal<PlannerBlock>();
	const [pending, setPending] = createSignal(false);
	const accountList = () => (accounts.error ? [] : (accounts() ?? []));
	const [title, setTitle] = createSignal("");
	const [description, setDescription] = createSignal("");
	const [ownerId, setOwnerId] = createSignal(0);
	const [branch, setBranch] = createSignal("");
	const [mrIid, setMrIid] = createSignal("");
	const [position, setPosition] = createSignal("0");
	const [parentId, setParentId] = createSignal("");

	function resetForm() {
		setTitle(""); setDescription(""); setOwnerId(0); setBranch(""); setMrIid(""); setPosition("0"); setParentId(""); setEditing(undefined);
	}
	function create() { resetForm(); setFormOpen(true); }
	function edit(block: PlannerBlock) {
		setEditing(block); setTitle(block.title); setDescription(block.description); setBranch(block.branch);
		setMrIid(block.mrIid?.toString() ?? ""); setPosition(block.position.toString()); setParentId(block.parentId?.toString() ?? "");
		setOwnerId(accountList().find((account) => account.username === block.owner.username)?.id ?? 0);
		setFormOpen(true);
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (pending() || !title().trim() || !ownerId()) return;
		setPending(true);
		const body = {
			title: title().trim(), description: description().trim(), ownerId: ownerId(), branch: branch().trim(),
			position: Number(position()) || 0,
			parentId: Number(parentId()) || 0,
			...(mrIid().trim() ? { mrIid: Number(mrIid()) } : {}),
		};
		try {
			const current = editing();
			if (current) {
				unwrap(await api.PUT("/api/admin/planner/blocks/{id}", {
					headers: csrfHeaders(props.csrf), params: { path: { id: current.id } }, body,
				}));
			} else {
				unwrap(await api.POST("/api/admin/planner/blocks", { headers: csrfHeaders(props.csrf), body }));
			}
			await refetch(); setFormOpen(false); resetForm(); toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	async function remove() {
		const block = deleting();
		if (!block || pending()) return;
		setPending(true);
		try {
			unwrap(await api.DELETE("/api/admin/planner/blocks/{id}", {
				headers: csrfHeaders(props.csrf), params: { path: { id: block.id } },
			}));
			await refetch(); setDeleting(undefined); toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div class={s.toolbar}>
				<div><h2>{m.plannerPageTitle()}</h2><p class={s.muted}>{m.plannerAdminHint()}</p></div>
				<Button variant="primary" onClick={create}>{m.createPlannerBlock()}</Button>
			</div>
			<Show when={!accounts.loading} fallback={<LoadingState />}>
				<Show when={!accounts.error} fallback={<ErrorState onRetry={() => void refetchAccounts()} />}>
					<Show when={!blocks.loading} fallback={<LoadingState />}>
						<Show when={!blocks.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
							<div class={s.grid}>
								<For each={blocks() ?? []} fallback={<EmptyState message={m.plannerEmpty()} />}>
									{(block) => (
										<article
											class={s.card}
											style={{ "--owner-color": safeColor(block.owner.color) }}
										>
											<div><h3>{block.title}</h3><p>{block.owner.displayName} · {plannerStatus(block.progress.state)}</p></div>
											<Show when={block.description}><p>{block.description}</p></Show>
											<Show when={block.branch}><p>{block.branch}</p></Show>
											<Show when={block.mrIid}><p>!{block.mrIid}</p></Show>
											<div class={s.actions}><Button onClick={() => edit(block)}>{m.edit()}</Button><Button variant="danger" onClick={() => setDeleting(block)}>{m.delete()}</Button></div>
										</article>
									)}
								</For>
							</div>
						</Show>
					</Show>
				</Show>
			</Show>

			<Dialog open={formOpen()} title={editing() ? m.editPlannerBlock() : m.createPlannerBlock()} onClose={() => { if (!pending()) setFormOpen(false); }}>
				<form class={s.form} onSubmit={save}>
					<Input label={m.plannerBlockTitle()} required maxlength={200} value={title()} onInput={(e) => setTitle(e.currentTarget.value)} />
					<label for="planner-description">{m.plannerBlockDescription()}</label>
					<textarea id="planner-description" rows={5} maxlength={20000} value={description()} onInput={(e) => setDescription(e.currentTarget.value)} />
					<fieldset><legend>{m.plannerOwner()}</legend><div class={s.choices}>
						<For each={accountList()}>{(account) => <Button data-selected={ownerId() === account.id} aria-pressed={ownerId() === account.id} style={{ "--owner-color": safeColor(account.color) }} onClick={() => setOwnerId(account.id)}>{account.displayName}</Button>}</For>
					</div></fieldset>
					<Input label={m.plannerParent()} type="number" min={0} value={parentId()} onInput={(e) => setParentId(e.currentTarget.value)} />
					<Input label={m.plannerBranch()} maxlength={255} value={branch()} onInput={(e) => setBranch(e.currentTarget.value)} />
					<Input label={m.mergeRequestIid()} type="number" min={1} value={mrIid()} onInput={(e) => setMrIid(e.currentTarget.value)} />
					<Input label={m.plannerPosition()} type="number" min={0} required value={position()} onInput={(e) => setPosition(e.currentTarget.value)} />
					<Button type="submit" variant="primary" disabled={pending() || !title().trim() || !ownerId()}>{pending() ? m.loading() : m.save()}</Button>
				</form>
			</Dialog>
			<ConfirmDialog open={!!deleting()} title={m.deletePlannerBlock()} description={deleting()?.title ?? ""} pending={pending()} danger onClose={() => { if (!pending()) setDeleting(undefined); }} onConfirm={() => void remove()} />
		</section>
	);
}

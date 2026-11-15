import { createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import {
	api,
	csrfHeaders,
	downloadProfile,
	unwrap,
} from "@/shared/api";
import type { components, paths } from "@/shared/api/schema";

type AdminDashboard = components["schemas"]["Dashboard"];
type PeerID = paths["/api/admin/peers/{id}"]["post"]["parameters"]["path"]["id"];
import { formatBytes, formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { ConfirmDialog } from "@/shared/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

type PeerChange = {
	id: PeerID;
	name: string;
	action: "enable" | "disable" | "rotate";
};

export function OverviewPanel(props: {
	dashboard: AdminDashboard;
	csrf?: string;
	onChange: () => void;
}) {
	const toast = useToast();
	const [offset, setOffset] = createSignal(0);
	const [change, setChange] = createSignal<PeerChange>();
	const [pending, setPending] = createSignal(false);
	const [profile, setProfile] = createSignal<{
		id: string;
		configuration: string;
	}>();
	const [usage, { refetch: refreshUsage }] = createResource(offset, async (value) =>
		unwrap(
			await api.GET("/api/admin/usage", {
				params: { query: { offset: value } },
			}),
		),
	);

	function requestPeer(id: string, name: string, action: PeerChange["action"]) {
		// Peer.id comes from the same server endpoint, whose path enum defines PeerID.
		setChange({ id: id as PeerID, name, action });
	}

	async function applyPeerChange() {
		const value = change();
		if (!value || pending()) return;
		setPending(true);
		try {
			const result = unwrap(
				await api.POST("/api/admin/peers/{id}", {
					headers: csrfHeaders(props.csrf),
					params: { path: { id: value.id } },
					body: { action: value.action },
				}),
			);
			if (result.configuration) {
				setProfile({ id: result.id, configuration: result.configuration });
			}
			setChange(undefined);
			props.onChange();
			toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<div class={s.panel}>
			<section class={s.section}>
				<div class={s.toolbar}>
					<h2>{m.servers()}</h2>
					<Button
						onClick={() => {
							props.onChange();
							void refreshUsage();
						}}
					>
						{m.retry()}
					</Button>
				</div>
				<div class={s.grid}>
					<For each={props.dashboard.servers ?? []} fallback={<EmptyState />}>
						{(server) => (
							<article class={s.card}>
								<h3>{server.name}</h3>
								<Show when={server.available} fallback={<p>{m.unavailable()}</p>}>
									<dl>
										<dt>{m.cpu()}</dt>
										<dd>{server.cpuPercent.toFixed(1)}%</dd>
										<dt>{m.memory()}</dt>
										<dd>
											{formatBytes(server.memoryUsed)} / {formatBytes(server.memoryTotal)}
										</dd>
										<dt>{m.disk()}</dt>
										<dd>
											{formatBytes(server.diskUsed)} / {formatBytes(server.diskTotal)}
										</dd>
									</dl>
									<time>{formatDate(server.sampledAt)}</time>
								</Show>
							</article>
						)}
					</For>
				</div>
			</section>

			<section class={s.section}>
				<h2>{m.adminContainers()}</h2>
				<div class={s.grid}>
					<For each={props.dashboard.containers ?? []} fallback={<EmptyState />}>
						{(container) => (
							<article class={s.card}>
								<h3>{container.name}</h3>
								<p>{container.server} · {container.image}</p>
								<p>{container.state} · {container.health}</p>
								<dl>
									<dt>{m.cpu()}</dt><dd>{container.cpuPercent.toFixed(1)}%</dd>
									<dt>{m.memory()}</dt><dd>{formatBytes(container.memoryUsed)} / {formatBytes(container.memoryLimit)}</dd>
									<dt>{m.containerRestarts()}</dt><dd>{container.restarts}</dd>
								</dl>
							</article>
						)}
					</For>
				</div>
			</section>

			<section class={s.section}>
				<h2>{m.vpn()}</h2>
				<Show when={props.dashboard.peerError}>
					<p role="alert">{m.unavailable()}</p>
				</Show>
				<div class={s.list}>
					<For each={props.dashboard.peers ?? []} fallback={<EmptyState />}>
						{(peer) => (
							<article class={s.row}>
								<div>
									<h3>{peer.name}</h3>
									<p>
										{peer.address} · {peer.enabled ? m.enabled() : m.disabled()}
									</p>
								</div>
								<Show when={!peer.protected} fallback={<p>{m.protected()}</p>}>
									<div class={s.actions}>
										<Button
											disabled={pending()}
											onClick={() =>
												requestPeer(
													peer.id,
													peer.name,
													peer.enabled ? "disable" : "enable",
												)
											}
										>
											{peer.enabled ? m.disable() : m.enable()}
										</Button>
										<Button
											disabled={pending()}
											onClick={() => requestPeer(peer.id, peer.name, "rotate")}
										>
											{m.rotate()}
										</Button>
									</div>
								</Show>
							</article>
						)}
					</For>
				</div>
				<Show when={profile()}>
					{(value) => (
						<div class={s.card}>
							<p>{m.rotateHint()}</p>
							<Button
								onClick={() => {
									downloadProfile(value().id, value().configuration);
									setProfile(undefined);
								}}
							>
								{m.download()}
							</Button>
						</div>
					)}
				</Show>
			</section>

			<section class={s.section}>
				<h2>{m.usage()}</h2>
				<p>{m.usageHint()}</p>
				<div class={s.grid}>
					<For each={props.dashboard.totals ?? []} fallback={<EmptyState />}>
						{(row) => (
							<article class={s.card}>
								<h3>{row.username}</h3>
								<p>
									{m.total()}: {row.totalTokens} · {m.requests()}: {row.requests} · {m.unknown()}: {row.unknownRequests}
								</p>
							</article>
						)}
					</For>
				</div>
			</section>

			<section class={s.section}>
				<h2>{m.audit()}</h2>
				<div class={s.list}>
					<For each={props.dashboard.audit ?? []} fallback={<EmptyState />}>
						{(row) => (
							<p class={s.row}>
								{formatDate(row.createdAt)} · {row.username} · {row.target} · {row.action} · {row.status}
							</p>
						)}
					</For>
				</div>
			</section>

			<section class={s.section}>
				<h2>{m.history()}</h2>
				<Show when={!usage.loading} fallback={<LoadingState />}>
					<Show when={!usage.error} fallback={<ErrorState onRetry={() => void refreshUsage()} />}>
						<For each={usage() ?? []} fallback={<EmptyState />}>
							{(row) => (
								<article class={s.card}>
									<p>{row.username} · {row.model} · {formatDate(row.createdAt)}</p>
									<p>
										{m.input()}: {row.inputTokens ?? m.unknown()} · {m.output()}: {row.outputTokens ?? m.unknown()} · {m.total()}: {row.totalTokens ?? m.unknown()}
									</p>
									<p>{row.status}</p>
								</article>
							)}
						</For>
					</Show>
				</Show>
				<div class={s.actions}>
					<Button
						disabled={offset() === 0 || usage.loading}
						onClick={() => setOffset((value) => Math.max(0, value - 100))}
					>
						{m.previous()}
					</Button>
					<Button
						disabled={usage.loading || (usage()?.length ?? 0) < 100}
						onClick={() => setOffset((value) => value + 100)}
					>
						{m.next()}
					</Button>
				</div>
			</section>

			<ConfirmDialog
				open={!!change()}
				title={m.confirmAction()}
				description={`${change()?.name ?? ""} · ${change()?.action ?? ""}`}
				pending={pending()}
				onClose={() => {
					if (!pending()) setChange(undefined);
				}}
				onConfirm={() => void applyPeerChange()}
			>
				<Show when={change()?.action === "rotate"}>
					<p>{m.rotateHint()}</p>
				</Show>
			</ConfirmDialog>
		</div>
	);
}

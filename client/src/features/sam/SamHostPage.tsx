import { A, useParams } from "@solidjs/router";
import { createEffect, createSignal, For, on, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate, formatNumber } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/Toast";
import type { SelectOption } from "@/shared/ui/Select";
import { SamBadge, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import { hostLicenseStatusLabel, hostLicenseStatusTone } from "./labels";
import {
	InstallationDialog,
	requestEdit,
	SamDeleteConfirm,
	SamEditActions,
	SamEditGate,
	SamLoginHint,
	useSamSession,
} from "./edit";

type Detail = components["schemas"]["SamComplianceHostDetail"];
type HostInstallation = components["schemas"]["SamComplianceHostInstallation"];
type Product = components["schemas"]["Product"];
type InstallationInput = components["schemas"]["InstallationInput"];

export default function SamHostPage() {
	const params = useParams();
	const { resource: detail, refetch } = createSamResource<Detail>(async () =>
		unwrap(
			await api.GET("/api/public/sam/compliance/hosts/{hostname}", {
				params: { path: { hostname: params.id ?? "" } },
			}),
		),
	);
	const { resource: products } = createSamResource<Product[]>(async () =>
		unwrap(await api.GET("/api/public/sam/catalog")) ?? [],
	);
	const session = useSamSession();
	const toast = useToast();
	createEffect(
		on(
			() => params.id,
			() => {
				void refetch();
			},
			{ defer: true },
		),
	);
	const title = () => detail()?.hostname ?? m.samHostPageTitle();
	const lead = () => m.samHostPageLead();

	const productOptions = (): SelectOption[] =>
		(products() ?? []).map((product) => ({
			value: String(product.id),
			label: product.name,
		}));

	const [formOpen, setFormOpen] = createSignal(false);
	const [formValue, setFormValue] = createSignal<HostInstallation>();
	const [pending, setPending] = createSignal(false);
	const [deleteTarget, setDeleteTarget] = createSignal<HostInstallation>();
	const [deletePending, setDeletePending] = createSignal(false);

	async function submit(input: InstallationInput) {
		setPending(true);
		try {
			const editing = formValue();
			if (editing?.id) {
				await api.PUT("/api/sam/installations/{id}", {
					params: { path: { id: editing.id } },
					body: input,
					headers: csrfHeaders(session.csrf()),
				});
			} else {
				await api.POST("/api/sam/installations", {
					body: input,
					headers: csrfHeaders(session.csrf()),
				});
			}
			setFormOpen(false);
			void refetch();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setPending(false);
		}
	}

	async function remove() {
		const target = deleteTarget();
		if (!target || deletePending()) return;
		setDeletePending(true);
		try {
			await api.DELETE("/api/sam/installations/{id}", {
				params: { path: { id: target.id } },
				headers: csrfHeaders(session.csrf()),
			});
			setDeleteTarget(undefined);
			void refetch();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setDeletePending(false);
		}
	}

	function hostFormValue() {
		const value = formValue();
		if (!value) return undefined;
		return {
			id: value.id,
			productId: value.productId ?? null,
			product: value.product,
			rawName: value.detectedName,
			deviceName: params.id ?? "",
			userName: "",
			version: value.version,
			source: "inventory" as const,
			status: value.status as InstallationInput["status"],
			lastSeenAt: new Date().toISOString(),
		};
	}

	return (
		<>
			<SamPageHeader title={title()} lead={lead()} />
			<A class={s.backLink} href="/sam/inventory">
				{m.samHostBack()}
			</A>
			<SamLoginHint loggedIn={session.loggedIn()} />
			<SamEditGate />
			<div class={s.addAction}>
			<Button
				variant="primary"
				onClick={() => {
					if (!requestEdit(session.loggedIn())) return;
					setFormValue(undefined);
					setFormOpen(true);
				}}
			>
				{m.samEditAddInstallation()}
			</Button>
			</div>
			<SamPageState data={detail} onRetry={() => void refetch()}>
				{(value) => (
					<div class={s.page}>
						<ul class={s.metrics}>
							<li class={s.metric}>
								<span class={s.metricValue}>{formatNumber(value.licensed)}</span>
								<span class={s.metricLabel}>{m.samHostLicensedCount()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>{formatNumber(value.expired)}</span>
								<span class={s.metricLabel}>{m.samHostExpiredCount()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>{formatNumber(value.unlicensed)}</span>
								<span class={s.metricLabel}>{m.samHostUnlicensedCount()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>{formatNumber(value.banned)}</span>
								<span class={s.metricLabel}>{m.samHostBannedCount()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>{formatNumber(value.inactive)}</span>
								<span class={s.metricLabel}>{m.samHostInactiveCount()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{value.lastSeenAt
										? formatDate(value.lastSeenAt)
										: m.samNeverSeen()}
								</span>
								<span class={s.metricLabel}>{m.samLastSeenAt()}</span>
							</li>
						</ul>

						<section class={s.section}>
							<h2 class={s.sectionTitle}>{m.samHostInstallationsTitle()}</h2>
							<Show
								when={(value.installations ?? []).length > 0}
								fallback={<p class={s.empty}>{m.samHostInstallationsEmpty()}</p>}
							>
								<ul class={s.list}>
									<For each={value.installations ?? []}>
										{(installation) => (
											<li class={s.row}>
												<div>
													<Show
														when={installation.productId}
														fallback={
															<span class={s.rowTitle}>
																{installation.detectedName}
															</span>
														}
													>
														{(id) => (
															<A
																class={s.inlineLink}
																href={`/sam/catalog/${id()}`}
															>
																{installation.product}
															</A>
														)}
													</Show>
													<p class={s.rowMeta}>
														{installation.vendor || m.samNotSpecified()}
														{installation.version
															? ` · ${installation.version}`
															: ""}
													</p>
												</div>
												<SamBadge
													label={hostLicenseStatusLabel(installation.status)}
													tone={hostLicenseStatusTone(installation.status)}
												/>
												<div class={s.rowTools}>
													<SamEditActions
														loggedIn={session.loggedIn()}
														editLabel={m.samEditEditInstallation()}
														deleteLabel={m.samEditDeleteInstallation()}
														onEdit={() => {
															setFormValue(installation);
															setFormOpen(true);
														}}
														onDelete={() => setDeleteTarget(installation)}
													/>
												</div>
											</li>
										)}
									</For>
								</ul>
							</Show>
						</section>
					</div>
				)}
			</SamPageState>
			<InstallationDialog
				open={formOpen()}
				title={
					formValue()?.id ? m.samEditEditInstallation() : m.samEditAddInstallation()
				}
				productOptions={productOptions()}
				deviceName={params.id ?? ""}
				value={hostFormValue()}
				pending={pending()}
				onClose={() => setFormOpen(false)}
				onSubmit={submit}
			/>
			<SamDeleteConfirm
				open={Boolean(deleteTarget())}
				title={`${m.samEditDeleteInstallation()}: ${deleteTarget()?.detectedName ?? ""}`}
				pending={deletePending()}
				onConfirm={remove}
				onClose={() => setDeleteTarget(undefined)}
			/>
		</>
	);
}

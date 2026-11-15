import { A, useParams } from "@solidjs/router";
import { createEffect, createResource, createSignal, For, on, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDay, formatMoney, formatNumber } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { useToast } from "@/shared/ui/Toast";
import { SamBadge, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import {
	hostLicenseStatusLabel,
	hostLicenseStatusTone,
	licenseKindLabel,
	licenseStatusLabel,
	licenseStatusTone,
	productRiskCostLabel,
	productStatusLabel,
	productStatusTone,
	riskLevelLabel,
	riskLevelTone,
} from "./labels";
import {
	ProductDialog as ProductFormDialog,
	requestEdit,
	SamAliasManager,
	SamEditGate,
	SamLoginHint,
	useSamSession,
} from "./edit";

type Detail = components["schemas"]["SamComplianceProductDetail"];
type ProductInput = components["schemas"]["ProductInput"];

export default function SamProductPage() {
	const params = useParams();
	const { resource: detail, refetch } = createSamResource<Detail>(async () =>
		unwrap(
			await api.GET("/api/public/sam/compliance/products/{id}", {
				params: { path: { id: Number(params.id) } },
			}),
		),
	);
	createEffect(
		on(
			() => params.id,
			() => {
				void refetch();
			},
			{ defer: true },
		),
	);
	const session = useSamSession();
	const toast = useToast();
	const [catalog] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/catalog")) ?? [],
	);
	const catalogProduct = () =>
		(catalog() ?? []).find((product) => product.id === Number(params.id));
	const [aliases, aliasesTools] = createResource(async () => {
		if (!session.loggedIn()) return [];
		try {
			return unwrap(
				await api.GET("/api/sam/products/{id}/aliases", {
					params: { path: { id: Number(params.id) } },
				}),
			)?.aliases ?? [];
		} catch {
			return [];
		}
	});
	const [formOpen, setFormOpen] = createSignal(false);
	const [pending, setPending] = createSignal(false);

	async function submitProduct(input: ProductInput) {
		setPending(true);
		try {
			await api.PUT("/api/sam/products/{id}", {
				params: { path: { id: Number(params.id) } },
				body: input,
				headers: csrfHeaders(session.csrf()),
			});
			setFormOpen(false);
			void refetch();
			void aliasesTools.refetch();
			toast.show(m.samEditSaved());
		} catch {
			toast.show(m.samEditFailed());
		} finally {
			setPending(false);
		}
	}

	const title = () => detail()?.product.name ?? m.samProductPageTitle();
	const lead = () => {
		const product = detail()?.product;
		if (!product) return m.samProductPageLead();
		return [product.vendor, product.category].filter(Boolean).join(" · ");
	};
	return (
		<>
			<SamPageHeader title={title()} lead={lead()} />
			<A class={s.backLink} href="/sam/compliance">
				{m.samProductBack()}
			</A>
			<SamLoginHint loggedIn={session.loggedIn()} />
			<SamEditGate />
			<Show when={catalogProduct()}>
				<Button
					variant="primary"
					onClick={() => {
						if (!requestEdit(session.loggedIn())) return;
						setFormOpen(true);
					}}
				>
					{m.samEditEditProduct()}
				</Button>
			</Show>
			<SamPageState data={detail} onRetry={() => void refetch()}>
				{(value) => (
					<div class={s.page}>
						<ul class={s.metricsWide}>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{formatNumber(value.product.purchasedSeats)}
								</span>
								<span class={s.metricLabel}>{m.samColumnPurchased()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{formatNumber(value.product.licensedSeats)}
								</span>
								<span class={s.metricLabel}>{m.samColumnActive()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{formatNumber(value.product.installedDevices)}
								</span>
								<span class={s.metricLabel}>{m.samColumnInstalled()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{value.product.deltaSeats > 0 ? "+" : ""}
									{formatNumber(value.product.deltaSeats)}
								</span>
								<span class={s.metricLabel}>{m.samColumnDelta()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{productRiskCostLabel(value.product)}
								</span>
								<span class={s.metricLabel}>{m.samColumnRisk()}</span>
							</li>
							<li class={s.metric}>
								<span class={s.metricValue}>
									{formatMoney(value.product.annualCost)}
								</span>
								<span class={s.metricLabel}>{m.samAnnualCost()}</span>
							</li>
						</ul>

						<div class={s.row}>
							<div class={s.rowHeader}>
								<span class={s.rowTitle}>{m.samRiskLevel()}</span>
								<SamBadge
									label={riskLevelLabel(value.product.riskLevel)}
									tone={riskLevelTone(value.product.riskLevel)}
								/>
							</div>
							<SamBadge
								label={productStatusLabel(value.product.status)}
								tone={productStatusTone(value.product.status)}
							/>
							<p class={s.rowMeta}>{value.product.recommendation}</p>
						</div>

						<section class={s.section}>
							<h2 class={s.sectionTitle}>{m.samProductLicensesTitle()}</h2>
							<Show
								when={(value.licenses ?? []).length > 0}
								fallback={<p class={s.empty}>{m.samProductLicensesEmpty()}</p>}
							>
								<ul class={s.list}>
									<For each={value.licenses ?? []}>
										{(license) => (
											<li class={s.row}>
												<div>
													<div class={s.rowHeader}>
														<span class={s.rowTitle}>
															{licenseKindLabel(license.kind)}
														</span>
														<SamBadge
															label={licenseStatusLabel(license.status)}
															tone={licenseStatusTone(license.status)}
														/>
													</div>
													<p class={s.rowMeta}>
														{license.validUntil
															? formatDay(license.validUntil)
															: m.samLicenseNoExpiry()}
													</p>
												</div>
												<dl
												classList={{
													[s.fields]: true,
													[s.fieldsWide]: true,
												}}
											>
													<div class={s.field}>
														<dt>{m.samLicenseSeats()}</dt>
														<dd>
															{license.seatsUsed} / {license.seatsTotal}
														</dd>
													</div>
													<div class={s.field}>
														<dt>{m.samCostPerSeat()}</dt>
														<dd>{formatMoney(license.costPerSeat)}</dd>
													</div>
												</dl>
											</li>
										)}
									</For>
								</ul>
							</Show>
						</section>

						<Show when={session.loggedIn()}>
							<section class={s.section}>
								<h2 class={s.sectionTitle}>{m.samEditFieldAlias()}</h2>
								<SamAliasManager
									loggedIn={session.loggedIn()}
									csrf={session.csrf()}
									productID={Number(params.id)}
									aliases={aliases() ?? []}
									onChange={() => void aliasesTools.refetch()}
								/>
							</section>
						</Show>

						<section class={s.section}>
							<h2 class={s.sectionTitle}>{m.samProductHostsTitle()}</h2>
							<Show
								when={(value.hosts ?? []).length > 0}
								fallback={<p class={s.empty}>{m.samProductHostsEmpty()}</p>}
							>
								<ul class={s.list}>
									<For each={value.hosts ?? []}>
										{(host) => (
											<li class={s.row}>
												<div>
													<A class={s.inlineLink} href={`/sam/inventory/${encodeURIComponent(host.hostname)}`}>
														{host.hostname}
													</A>
													<p class={s.rowMeta}>
														{host.version || m.samNotSpecified()}
													</p>
												</div>
												<SamBadge
													label={hostLicenseStatusLabel(host.licenseStatus)}
													tone={hostLicenseStatusTone(host.licenseStatus)}
												/>
											</li>
										)}
									</For>
								</ul>
							</Show>
						</section>
					</div>
				)}
			</SamPageState>
			<ProductFormDialog
				open={formOpen()}
				title={m.samEditEditProduct()}
				value={catalogProduct()}
				pending={pending()}
				onClose={() => setFormOpen(false)}
				onSubmit={submitProduct}
			/>
		</>
	);
}

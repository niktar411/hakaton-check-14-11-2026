import { A } from "@solidjs/router";
import { createMemo, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatMoney, formatNumber } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { Select } from "@/shared/ui/Select";
import { SamBadge, SamPageHeader } from "./SamPage";
import s from "./SamCompliancePage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import {
	productRiskCostLabel,
	productStatusLabel,
	productStatusTone,
} from "./labels";

type Summary = components["schemas"]["SamComplianceSummary"];
type Product = components["schemas"]["SamComplianceProduct"];
type Risk = components["schemas"]["SamComplianceRisk"];

type ComplianceData = {
	summary: Summary;
	products: Product[];
	risks: Risk[];
};

type StateFilter = "all" | "over" | "under" | "banned" | "expired";

function isEmpty(value: ComplianceData) {
	return value.summary.products === 0 && value.products.length === 0;
}

function productStates(product: Product) {
	const states: Array<{ label: string; tone: "danger" | "warning" | "success" }> = [];
	if (product.status === "banned" && product.bannedInstallations > 0) {
		states.push({ label: m.samStateBanned(), tone: "danger" });
	}
	if (product.overusedSeats > 0) {
		states.push({ label: m.samStateOver(), tone: "danger" });
	}
	if (product.expiredSeats > 0) {
		states.push({ label: m.samStateExpired(), tone: "warning" });
	}
	if (product.idleSeats > 0) {
		states.push({ label: m.samStateUnder(), tone: "warning" });
	}
	if (states.length === 0) {
		states.push({ label: m.samStateOk(), tone: "success" });
	}
	return states;
}

function rowTone(product: Product) {
	if (product.status === "banned" && product.bannedInstallations > 0) return "banned";
	if (product.overusedSeats > 0) return "over";
	if (product.expiredSeats > 0) return "expired";
	if (product.idleSeats > 0) return "under";
	return "ok";
}

function matchesState(product: Product, filter: StateFilter) {
	switch (filter) {
		case "over":
			return product.overusedSeats > 0;
		case "under":
			return product.idleSeats > 0;
		case "banned":
			return product.status === "banned" && product.bannedInstallations > 0;
		case "expired":
			return product.expiredSeats > 0;
		default:
			return true;
	}
}

function FilterGroup(props: {
	label: string;
	value: string;
	options: Array<{ value: string; label: string }>;
	onChange: (value: string) => void;
}) {
	return (
		<div class={s.filterGroup}>
			<span class={s.filterLabel}>{props.label}</span>
			<div class={s.filterOptions}>
				<For each={props.options}>
					{(option) => (
						<Button
							variant={props.value === option.value ? "primary" : "secondary"}
							aria-pressed={props.value === option.value}
							onClick={() => props.onChange(option.value)}
						>
							{option.label}
						</Button>
					)}
				</For>
			</div>
		</div>
	);
}

export default function SamCompliancePage() {
	const { resource: data, refetch } = createSamResource<ComplianceData>(async () => {
		const [summary, products, risks] = await Promise.all([
			api.GET("/api/public/sam/compliance/summary"),
			api.GET("/api/public/sam/compliance/products"),
			api.GET("/api/public/sam/compliance/risks"),
		]);
		return {
			summary: unwrap(summary),
			products: unwrap(products) ?? [],
			risks: unwrap(risks) ?? [],
		};
	});
	const [category, setCategory] = createSignal("all");
	const [risk, setRisk] = createSignal("all");
	const [state, setState] = createSignal<StateFilter>("all");

	const categories = createMemo(() => {
		const value = data();
		if (!value) return [];
		return Array.from(new Set(value.products.map((item) => item.category)))
			.filter(Boolean)
			.sort((left, right) => left.localeCompare(right, "ru"));
	});

	const filteredProducts = createMemo(() => {
		const value = data();
		if (!value) return [];
		return value.products.filter(
			(product) =>
				(category() === "all" || product.category === category()) &&
				(risk() === "all" || product.riskLevel === risk()) &&
				matchesState(product, state()),
		);
	});

	return (
		<>
			<SamPageHeader
				title={m.samCompliancePageTitle()}
				lead={m.samComplianceLead()}
			/>
			<SamPageState
				data={data}
				empty={isEmpty}
				onRetry={() => void refetch()}
				emptyMessage={m.samComplianceEmpty()}
			>
				{(value) => (
					<div class={s.page}>
						<section class={s.kpis} aria-label={m.samComplianceSummaryTitle()}>
							<div class={s.kpi} classList={{ [s.toneBad]: true }}>
								<span class={s.kpiValue}>{formatMoney(value.summary.costAtRisk)}</span>
								<span class={s.kpiLabel}>{m.samCostAtRisk()}</span>
							</div>
							<div
								class={s.kpi}
								classList={{
									[s.toneGood]: value.summary.compliancePercent >= 80,
									[s.toneWarn]: value.summary.compliancePercent < 80,
								}}
							>
								<span class={s.kpiValue}>{value.summary.compliancePercent}%</span>
								<span class={s.kpiLabel}>{m.samCompliancePercent()}</span>
							</div>
							<div class={s.kpi} classList={{ [s.toneBad]: true }}>
								<span class={s.kpiValue}>
									{formatNumber(value.summary.overusedSeats)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiOverused()}</span>
							</div>
							<div class={s.kpi} classList={{ [s.toneWarn]: true }}>
								<span class={s.kpiValue}>
									{formatNumber(value.summary.idleSeats)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiIdleSeats()}</span>
							</div>
							<div class={s.kpi} classList={{ [s.toneWarn]: true }}>
								<span class={s.kpiValue}>
									{formatNumber(value.summary.expiredSeats)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiExpiredSeats()}</span>
							</div>
							<div class={s.kpi} classList={{ [s.toneBad]: true }}>
								<span class={s.kpiValue}>
									{formatNumber(value.summary.bannedInstallations)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiBanned()}</span>
							</div>
						</section>

						<section class={s.section}>
							<div class={s.sectionHeader}>
								<div>
									<h2>{m.samBalanceTitle()}</h2>
									<p>{m.samBalanceHint()}</p>
								</div>
							</div>
							<div class={s.filters}>
								<div class={s.filterGroup}>
									<span class={s.filterLabel}>{m.samFilterCategory()}</span>
									<Select
										label={m.samFilterCategory()}
										value={category()}
										onChange={setCategory}
										options={[
											{ value: "all", label: m.samFilterAll() },
											...categories().map((item) => ({
												value: item,
												label: item,
											})),
										]}
									/>
								</div>
								<FilterGroup
									label={m.samFilterRisk()}
									value={risk()}
									onChange={setRisk}
									options={[
										{ value: "all", label: m.samFilterAll() },
										{ value: "high", label: m.samRiskLevelHigh() },
										{ value: "medium", label: m.samRiskLevelMedium() },
										{ value: "low", label: m.samRiskLevelLow() },
									]}
								/>
								<FilterGroup
									label={m.samFilterState()}
									value={state()}
									onChange={(next) => setState(next as StateFilter)}
									options={[
										{ value: "all", label: m.samFilterAll() },
										{ value: "over", label: m.samStateOver() },
										{ value: "under", label: m.samStateUnder() },
										{ value: "banned", label: m.samStateBanned() },
										{ value: "expired", label: m.samStateExpired() },
									]}
								/>
							</div>
							<Show
								when={filteredProducts().length > 0}
								fallback={<p class={s.empty}>{m.samNoFilteredProducts()}</p>}
							>
								<div class={s.tableWrap}>
									<table class={s.table}>
										<thead>
											<tr>
												<th scope="col">{m.samColumnProduct()}</th>
												<th scope="col">{m.samColumnCategory()}</th>
												<th scope="col">{m.samColumnPurchased()}</th>
												<th scope="col">{m.samColumnActive()}</th>
												<th scope="col">{m.samColumnInstalled()}</th>
												<th scope="col">{m.samColumnDelta()}</th>
												<th scope="col">{m.samColumnRisk()}</th>
												<th scope="col">{m.samColumnState()}</th>
												<th scope="col">{m.samColumnRecommendation()}</th>
											</tr>
										</thead>
										<tbody>
											<For each={filteredProducts()}>
												{(product) => (
													<tr
														classList={{
															[s.rowBanned]: rowTone(product) === "banned",
															[s.rowOver]: rowTone(product) === "over",
															[s.rowExpired]: rowTone(product) === "expired",
															[s.rowUnder]: rowTone(product) === "under",
														}}
													>
														<th scope="row">
															<span class={s.mobileLabel}>
																{m.samColumnProduct()}
															</span>
															<span class={s.productCell}>
																<A
																	class={s.productLink}
																	href={`/sam/catalog/${product.id}`}
																>
																	{product.name}
																</A>
																<span class={s.productMeta}>
																	{product.vendor}
																</span>
																<SamBadge
																	label={productStatusLabel(product.status)}
																	tone={productStatusTone(product.status)}
																/>
															</span>
														</th>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnCategory()}
															</span>
															{product.category}
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnPurchased()}
															</span>
															{formatNumber(product.purchasedSeats)}
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnActive()}
															</span>
															{formatNumber(product.licensedSeats)}
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnInstalled()}
															</span>
															{formatNumber(product.installedDevices)}
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnDelta()}
															</span>
															<span
																classList={{
																	[s.negative]: product.deltaSeats > 0,
																	[s.positive]: product.deltaSeats < 0,
																}}
															>
																{product.deltaSeats > 0 ? "+" : ""}
																{formatNumber(product.deltaSeats)}
															</span>
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnRisk()}
															</span>
															{productRiskCostLabel(product)}
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnState()}
															</span>
															<span class={s.states}>
																<For each={productStates(product)}>
																	{(item) => (
																		<SamBadge
																			label={item.label}
																			tone={item.tone}
																		/>
																	)}
																</For>
															</span>
														</td>
														<td>
															<span class={s.mobileLabel}>
																{m.samColumnRecommendation()}
															</span>
															{product.recommendation}
														</td>
													</tr>
												)}
											</For>
										</tbody>
									</table>
								</div>
							</Show>
						</section>
					</div>
				)}
			</SamPageState>
		</>
	);
}

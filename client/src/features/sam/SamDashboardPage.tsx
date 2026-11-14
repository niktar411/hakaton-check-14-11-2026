import { A } from "@solidjs/router";
import { createMemo, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatMoney, formatNumber } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { SamBadge, SamPageHeader } from "./SamPage";
import s from "./SamDashboardPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import {
	productStatusLabel,
	productStatusTone,
	productRiskCostLabel,
	riskCodeLabel,
	riskCostLabel,
	riskLevelLabel,
	riskLevelTone,
} from "./labels";

type Summary = components["schemas"]["SamComplianceSummary"];
type Product = components["schemas"]["SamComplianceProduct"];
type Risk = components["schemas"]["SamComplianceRisk"];

type DashboardData = {
	summary: Summary;
	products: Product[];
	risks: Risk[];
};

function isEmpty(value: DashboardData) {
	return (
		value.summary.products === 0 &&
		value.summary.devices === 0 &&
		value.summary.licenses === 0 &&
		value.summary.installations === 0
	);
}

function barWidth(value: number, max: number) {
	if (max <= 0 || value <= 0) return "0%";
	return `${Math.max(6, Math.round((value / max) * 100))}%`;
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

export default function SamDashboardPage() {
	const { resource: data, refetch } = createSamResource<DashboardData>(async () => {
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
				(risk() === "all" || product.riskLevel === risk()),
		);
	});

	const filteredRisks = createMemo(() => {
		const value = data();
		if (!value) return [];
		const products = new Set(filteredProducts().map((product) => product.id));
		return value.risks.filter(
			(item) => item.productId === undefined || products.has(item.productId),
		);
	});

	const maxStatus = createMemo(() => {
		const value = data();
		if (!value) return 0;
		return Math.max(...(value.summary.statuses ?? []).map((item) => item.products), 0);
	});

	const maxCategory = createMemo(() => {
		const value = data();
		if (!value) return 0;
		return Math.max(
			...(value.summary.categories ?? []).map((item) => item.installedDevices),
			0,
		);
	});

	return (
		<>
			<SamPageHeader
				title={m.samDashboardPageTitle()}
				lead={m.samDashboardLead()}
			/>
			<SamPageState
				data={data}
				empty={isEmpty}
				onRetry={() => void refetch()}
				emptyMessage={m.samDashboardEmpty()}
			>
				{(value) => (
					<div class={s.page}>
						<section class={s.kpis} aria-label={m.samDashboardPageTitle()}>
							<A class={s.kpi} href="/sam/inventory">
								<span class={s.kpiValue}>{formatNumber(value.summary.devices)}</span>
								<span class={s.kpiLabel}>{m.samKpiHosts()}</span>
							</A>
							<A class={s.kpi} href="/sam/catalog">
								<span class={s.kpiValue}>
									{formatNumber(value.summary.products)}
								</span>
								<span class={s.kpiLabel}>{m.samMetricProducts()}</span>
							</A>
							<A class={s.kpi} href="/sam/licenses">
								<span class={s.kpiValue}>
									{formatNumber(value.summary.licenses)}
								</span>
								<span class={s.kpiLabel}>{m.samMetricLicenses()}</span>
							</A>
							<A class={s.kpi} href="/sam/compliance">
								<span class={s.kpiValue}>
									{value.summary.compliancePercent}%
								</span>
								<span class={s.kpiLabel}>{m.samKpiCompliance()}</span>
								<span class={s.kpiHint}>{m.samCompliancePercentHint()}</span>
							</A>
							<A class={s.kpi} href="/sam/compliance">
								<span class={s.kpiValue}>
									{formatMoney(value.summary.costAtRisk)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiRisk()}</span>
								<span class={s.kpiHint}>{m.samCostAtRiskHint()}</span>
							</A>
							<div class={s.kpi}>
								<span class={s.kpiValue}>
									{formatMoney(value.summary.idleCost)}
								</span>
								<span class={s.kpiLabel}>{m.samKpiIdle()}</span>
								<span class={s.kpiHint}>{m.samIdleCost()}</span>
							</div>
						</section>

						<section class={s.riskBanner}>
							<div class={s.riskBannerMain}>
								<span class={s.riskBannerLabel}>{m.samRiskLevel()}</span>
								<SamBadge
									label={riskLevelLabel(value.summary.riskLevel)}
									tone={riskLevelTone(value.summary.riskLevel)}
								/>
								<p class={s.riskBannerHint}>
									{m.samAnnualCost()}: {formatMoney(value.summary.annualCost)}
								</p>
							</div>
							<div class={s.riskBannerNumbers}>
								<div>
									<span>{m.samKpiOverused()}</span>
									<strong>{formatNumber(value.summary.overusedSeats)}</strong>
								</div>
								<div>
									<span>{m.samKpiIdleSeats()}</span>
									<strong>{formatNumber(value.summary.idleSeats)}</strong>
								</div>
								<div>
									<span>{m.samKpiExpiredSeats()}</span>
									<strong>{formatNumber(value.summary.expiredSeats)}</strong>
								</div>
								<div>
									<span>{m.samKpiBanned()}</span>
									<strong>
										{formatNumber(value.summary.bannedInstallations)}
									</strong>
								</div>
								<div>
									<span>{m.samKpiUnmatched()}</span>
									<strong>
										{formatNumber(value.summary.unmatchedInstallations)}
									</strong>
								</div>
							</div>
							<div class={s.recommendations}>
								<h2>{m.samRecommendations()}</h2>
								<ul>
									<For each={value.summary.recommendations ?? []}>
										{(item) => <li>{item}</li>}
									</For>
								</ul>
							</div>
						</section>

						<section class={s.charts}>
							<article class={s.chart}>
								<h2>{m.samChartStatuses()}</h2>
								<Show
									when={maxStatus() > 0}
									fallback={<p class={s.chartEmpty}>{m.samChartEmpty()}</p>}
								>
									<ul class={s.bars}>
										<For each={value.summary.statuses ?? []}>
											{(item) => (
												<li class={s.barRow}>
													<span class={s.barLabel}>
														{productStatusLabel(item.status)}
													</span>
													<span class={s.barTrack} aria-hidden="true">
														<span
															class={s.barFill}
															style={{
																width: barWidth(item.products, maxStatus()),
															}}
														/>
													</span>
													<span class={s.barValue}>
														{formatNumber(item.products)}
													</span>
												</li>
											)}
										</For>
									</ul>
								</Show>
							</article>
							<article class={s.chart}>
								<h2>{m.samChartCategories()}</h2>
								<Show
									when={maxCategory() > 0}
									fallback={<p class={s.chartEmpty}>{m.samChartEmpty()}</p>}
								>
									<ul class={s.bars}>
										<For each={value.summary.categories ?? []}>
											{(item) => (
												<li class={s.barRow}>
													<span class={s.barLabel}>{item.category}</span>
													<span class={s.barTrack} aria-hidden="true">
														<span
															class={s.barFill}
															style={{
																width: barWidth(
																	item.installedDevices,
																	maxCategory(),
																),
															}}
														/>
													</span>
													<span class={s.barValue}>
														{formatNumber(item.installedDevices)}
													</span>
												</li>
											)}
										</For>
									</ul>
								</Show>
							</article>
						</section>

						<section class={s.section}>
							<div class={s.sectionHeader}>
								<div>
									<h2>{m.samTopRisks()}</h2>
									<p>{m.samCostAtRiskHint()}</p>
								</div>
							</div>
							<div class={s.filters}>
								<FilterGroup
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
							</div>
							<Show
								when={filteredRisks().length > 0}
								fallback={<p class={s.empty}>{m.samNoRisks()}</p>}
							>
								<ul class={s.riskList}>
									<For each={filteredRisks().slice(0, 6)}>
										{(item) => (
											<li class={s.riskItem}>
												<div class={s.riskItemMain}>
													<div class={s.riskItemHeader}>
														<SamBadge
															label={riskCodeLabel(item.code)}
															tone={
																item.severity === "high" ? "danger" : "warning"
															}
														/>
														<Show
															when={item.productId}
															fallback={
																<A class={s.riskLink} href="/sam/matching">
																	{m.samMatchingPageTitle()}
																</A>
															}
														>
															{(id) => (
																<A
																	class={s.riskLink}
																	href={`/sam/catalog/${id()}`}
																>
																	{item.productName}
																</A>
															)}
														</Show>
													</div>
													<p class={s.riskRecommendation}>
														{item.recommendation}
													</p>
												</div>
												<dl class={s.riskNumbers}>
													<div>
														<dt>{m.samRiskAffected()}</dt>
														<dd>{formatNumber(item.affected)}</dd>
													</div>
													<div>
														<dt>{m.samRiskCost()}</dt>
														<dd>{riskCostLabel(item.cost)}</dd>
													</div>
												</dl>
											</li>
										)}
									</For>
								</ul>
							</Show>
						</section>

						<section class={s.section}>
							<div class={s.sectionHeader}>
								<div>
									<h2>{m.samBalanceTitle()}</h2>
									<p>{m.samBalanceHint()}</p>
								</div>
								<A class={s.sectionLink} href="/sam/compliance">
									{m.samCompliancePageTitle()}
								</A>
							</div>
							<Show
								when={filteredProducts().length > 0}
								fallback={<p class={s.empty}>{m.samNoFilteredProducts()}</p>}
							>
								<ul class={s.productList}>
									<For each={filteredProducts().slice(0, 8)}>
										{(product) => (
											<li class={s.productItem}>
												<div class={s.productMain}>
													<A class={s.productLink} href={`/sam/catalog/${product.id}`}>
														{product.name}
													</A>
													<span class={s.productMeta}>
														{product.vendor} · {product.category}
													</span>
												</div>
												<SamBadge
													label={productStatusLabel(product.status)}
													tone={productStatusTone(product.status)}
												/>
												<dl class={s.productNumbers}>
													<div>
														<dt>{m.samColumnPurchased()}</dt>
														<dd>{formatNumber(product.purchasedSeats)}</dd>
													</div>
													<div>
														<dt>{m.samColumnInstalled()}</dt>
														<dd>{formatNumber(product.installedDevices)}</dd>
													</div>
													<div>
														<dt>{m.samColumnDelta()}</dt>
														<dd
															classList={{
																[s.negative]: product.deltaSeats > 0,
																[s.positive]: product.deltaSeats < 0,
															}}
														>
															{product.deltaSeats > 0 ? "+" : ""}
															{formatNumber(product.deltaSeats)}
														</dd>
													</div>
													<div>
														<dt>{m.samColumnRisk()}</dt>
														<dd>{productRiskCostLabel(product)}</dd>
													</div>
												</dl>
											</li>
										)}
									</For>
								</ul>
							</Show>
						</section>
					</div>
				)}
			</SamPageState>
		</>
	);
}

import { createResource, For } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { SamPageState } from "./SamPageState";

type Overview = components["schemas"]["Overview"];

function metrics(value: Overview) {
	return [
		{ label: m.samMetricProducts(), value: String(value.products) },
		{ label: m.samMetricAssets(), value: String(value.assets) },
		{ label: m.samMetricLicenses(), value: String(value.licenses) },
		{
			label: m.samMetricSeats(),
			value: `${value.seatsUsed} / ${value.seatsTotal}`,
		},
		{ label: m.samMetricOpenFindings(), value: String(value.openFindings) },
		{
			label: m.samMetricPendingMatches(),
			value: String(value.pendingMatches),
		},
		{
			label: m.samMetricExpiringLicenses(),
			value: String(value.expiringLicenses),
		},
		{
			label: m.samMetricUnlicensedInstallations(),
			value: String(value.unlicensedInstallations),
		},
	];
}

function isEmpty(value: Overview) {
	return (
		value.products === 0 &&
		value.assets === 0 &&
		value.licenses === 0 &&
		value.openFindings === 0 &&
		value.pendingMatches === 0 &&
		value.unlicensedInstallations === 0
	);
}

export default function SamDashboardPage() {
	const [overview, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/dashboard")),
	);
	return (
		<>
			<SamPageHeader
				title={m.samDashboardPageTitle()}
				lead={m.samOverviewLead()}
			/>
			<SamPageState
				data={overview}
				empty={isEmpty}
				onRetry={() => void refetch()}
				emptyMessage={m.samDashboardEmpty()}
			>
				{(value) => (
					<ul class={s.metrics}>
						<For each={metrics(value)}>
							{(metric) => (
								<li class={s.metric}>
									<span class={s.metricValue}>{metric.value}</span>
									<span class={s.metricLabel}>{metric.label}</span>
								</li>
							)}
						</For>
					</ul>
				)}
			</SamPageState>
		</>
	);
}

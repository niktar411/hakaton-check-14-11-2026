import { createResource, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import {
	SamBadge,
	SamFields,
	SamList,
	SamPageHeader,
} from "./SamPage";
import s from "./SamPage.module.css";
import { SamPageState } from "./SamPageState";
import {
	findingSeverityLabel,
	findingSeverityTone,
	findingStatusLabel,
} from "./labels";

type Finding = components["schemas"]["Finding"];

export default function SamCompliancePage() {
	const [findings, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/compliance")) ?? [],
	);
	return (
		<>
			<SamPageHeader
				title={m.samCompliancePageTitle()}
				lead={m.samComplianceLead()}
			/>
			<SamPageState
				data={findings}
				empty={(value: Finding[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samComplianceEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(finding) => (
								<li class={s.row}>
									<div>
										<div class={s.rowHeader}>
											<h2 class={s.rowTitle}>{finding.code}</h2>
											<SamBadge
												label={findingSeverityLabel(finding.severity)}
												tone={findingSeverityTone(finding.severity)}
											/>
										</div>
										<Show when={finding.details}>
											<p class={s.rowMeta}>{finding.details}</p>
										</Show>
									</div>
									<SamFields
										items={[
											{
												label: m.samFindingStatus(),
												value: findingStatusLabel(finding.status),
											},
											{
												label: m.samFindingProduct(),
												value: finding.product || m.samNotSpecified(),
											},
											{
												label: m.samFindingAsset(),
												value: finding.asset || m.samNotSpecified(),
											},
											{
												label: m.samFindingDetectedAt(),
												value: formatDate(finding.detectedAt),
											},
										]}
									/>
								</li>
							)}
						</For>
					</SamList>
				)}
			</SamPageState>
		</>
	);
}

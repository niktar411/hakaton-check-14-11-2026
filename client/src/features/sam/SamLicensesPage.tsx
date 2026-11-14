import { For } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDay } from "@/shared/lib/format";
import {
	SamBadge,
	SamFields,
	SamList,
	SamPageHeader,
} from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import {
	licenseKindLabel,
	licenseStatusLabel,
	licenseStatusTone,
} from "./labels";

type License = components["schemas"]["License"];

export default function SamLicensesPage() {
	const { resource: licenses, refetch } = createSamResource<License[]>(async () =>
		unwrap(await api.GET("/api/public/sam/licenses")) ?? [],
	);
	return (
		<>
			<SamPageHeader
				title={m.samLicensesPageTitle()}
				lead={m.samLicensesLead()}
			/>
			<SamPageState
				data={licenses}
				empty={(value: License[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samLicensesEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(license) => (
								<li class={s.row}>
									<div>
										<div class={s.rowHeader}>
											<h2 class={s.rowTitle}>{license.product}</h2>
											<SamBadge
												label={licenseStatusLabel(license.status)}
												tone={licenseStatusTone(license.status)}
											/>
										</div>
										<p class={s.rowMeta}>{license.vendor}</p>
									</div>
									<SamFields
										items={[
											{
												label: m.samLicenseKind(),
												value: licenseKindLabel(license.kind),
											},
											{
												label: m.samLicenseSeats(),
												value: `${license.seatsUsed} / ${license.seatsTotal}`,
											},
											{
												label: m.samLicenseValidUntil(),
												value: license.validUntil
													? formatDay(license.validUntil)
													: m.samLicenseNoExpiry(),
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

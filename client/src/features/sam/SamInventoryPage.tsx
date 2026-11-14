import { A } from "@solidjs/router";
import { createResource, For } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { SamFields, SamList, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { SamPageState } from "./SamPageState";

type Asset = components["schemas"]["Asset"];

export default function SamInventoryPage() {
	const [assets, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/inventory")) ?? [],
	);
	return (
		<>
			<SamPageHeader
				title={m.samInventoryPageTitle()}
				lead={m.samInventoryLead()}
			/>
			<SamPageState
				data={assets}
				empty={(value: Asset[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samInventoryEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(asset) => (
								<li class={s.row}>
									<div>
										<h2 class={s.rowTitle}>
											<A class={s.inlineLink} href={`/sam/inventory/${encodeURIComponent(asset.hostname)}`}>
												{asset.hostname}
											</A>
										</h2>
										<p class={s.rowMeta}>{asset.platform}</p>
									</div>
									<SamFields
										items={[
											{
												label: m.samOwner(),
												value: asset.owner || m.samNotSpecified(),
											},
											{
												label: m.samSoftwareCount(),
												value: String(asset.softwareCount),
											},
											{
												label: m.samLastSeenAt(),
												value: asset.lastSeenAt
													? formatDate(asset.lastSeenAt)
													: m.samNeverSeen(),
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

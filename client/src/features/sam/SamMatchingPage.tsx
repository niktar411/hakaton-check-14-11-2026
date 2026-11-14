import { createResource, For } from "solid-js";
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
import { matchStatusLabel, matchStatusTone } from "./labels";

type Match = components["schemas"]["Match"];

export default function SamMatchingPage() {
	const [matches, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/matching")) ?? [],
	);
	return (
		<>
			<SamPageHeader
				title={m.samMatchingPageTitle()}
				lead={m.samMatchingLead()}
			/>
			<SamPageState
				data={matches}
				empty={(value: Match[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samMatchingEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(match) => (
								<li class={s.row}>
									<div>
										<div class={s.rowHeader}>
											<h2 class={s.rowTitle}>{match.detectedName}</h2>
											<SamBadge
												label={matchStatusLabel(match.status)}
												tone={matchStatusTone(match.status)}
											/>
										</div>
										<p class={s.rowMeta}>
											{m.samVersion()}: {match.version || m.samNotSpecified()}
										</p>
									</div>
									<SamFields
										items={[
											{
												label: m.samMatchAsset(),
												value: match.asset,
											},
											{
												label: m.samMatchProduct(),
												value: match.product || m.samMatchUnlinked(),
											},
											{
												label: m.samMatchConfidence(),
												value: `${Math.round(match.confidence * 100)}%`,
											},
											{
												label: m.samMatchCreatedAt(),
												value: formatDate(match.createdAt),
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

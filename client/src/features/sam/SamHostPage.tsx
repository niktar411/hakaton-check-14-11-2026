import { A, useParams } from "@solidjs/router";
import { createEffect, For, on, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate, formatNumber } from "@/shared/lib/format";
import { SamBadge, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";
import { hostLicenseStatusLabel, hostLicenseStatusTone } from "./labels";

type Detail = components["schemas"]["HostDetail"];

export default function SamHostPage() {
	const params = useParams();
	const { resource: detail, refetch } = createSamResource<Detail>(async () =>
		unwrap(
			await api.GET("/api/public/sam/compliance/hosts/{id}", {
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
	const title = () => detail()?.hostname ?? m.samHostPageTitle();
	const lead = () => {
		const host = detail();
		if (!host) return m.samHostPageLead();
		return [host.platform, host.owner || m.samNotSpecified()].join(" · ");
	};
	return (
		<>
			<SamPageHeader title={title()} lead={lead()} />
			<A class={s.backLink} href="/sam/inventory">
				{m.samHostBack()}
			</A>
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

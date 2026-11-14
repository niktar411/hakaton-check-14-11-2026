import { A } from "@solidjs/router";
import { For } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { formatDate } from "@/shared/lib/format";
import { SamFields, SamList, SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { createSamResource, SamPageState } from "./SamPageState";

type Product = components["schemas"]["Product"];

export default function SamCatalogPage() {
	const { resource: products, refetch } = createSamResource<Product[]>(async () =>
		unwrap(await api.GET("/api/public/sam/catalog")) ?? [],
	);
	return (
		<>
			<SamPageHeader
				title={m.samCatalogPageTitle()}
				lead={m.samCatalogLead()}
			/>
			<SamPageState
				data={products}
				empty={(value: Product[]) => value.length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samCatalogEmpty()}
			>
				{(value) => (
					<SamList>
						<For each={value}>
							{(product) => (
								<li class={s.row}>
									<div>
										<h2 class={s.rowTitle}>
											<A class={s.inlineLink} href={`/sam/catalog/${product.id}`}>
												{product.name}
											</A>
										</h2>
										<p class={s.rowMeta}>
											{product.vendor} · {product.category}
										</p>
									</div>
									<SamFields
										items={[
											{
												label: m.samLicenseCount(),
												value: String(product.licenseCount),
											},
											{
												label: m.samInstallationCount(),
												value: String(product.installationCount),
											},
											{
												label: m.samUpdatedAt(),
												value: formatDate(product.updatedAt),
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

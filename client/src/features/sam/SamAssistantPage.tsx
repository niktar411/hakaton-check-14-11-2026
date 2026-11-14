import { A } from "@solidjs/router";
import { createResource, For } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { SamPageHeader } from "./SamPage";
import s from "./SamPage.module.css";
import { SamPageState } from "./SamPageState";
import { suggestionLabel } from "./labels";

type Assistant = components["schemas"]["Assistant"];

export default function SamAssistantPage() {
	const [assistant, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/public/sam/assistant")),
	);
	return (
		<>
			<SamPageHeader
				title={m.samAssistantPageTitle()}
				lead={m.samAssistantLead()}
			/>
			<SamPageState
				data={assistant}
				empty={(value: Assistant) => (value.suggestions ?? []).length === 0}
				onRetry={() => void refetch()}
				emptyMessage={m.samAssistantEmpty()}
			>
				{(value) => (
					<ul class={s.suggestions}>
						<For each={value.suggestions ?? []}>
							{(suggestion) => (
								<li class={s.suggestion}>
									<div class={s.suggestionText}>
										<p class={s.suggestionLabel}>
											{suggestionLabel(suggestion.kind)}
										</p>
										<p class={s.suggestionCount}>{suggestion.count}</p>
									</div>
									<A class={s.suggestionLink} href={suggestion.path}>
										{m.samOpenSuggestion()}
									</A>
								</li>
							)}
						</For>
					</ul>
				)}
			</SamPageState>
		</>
	);
}

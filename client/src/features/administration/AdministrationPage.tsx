import { useNavigate } from "@solidjs/router";
import { createEffect, createResource, createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, identity, isAuthError, unwrap } from "@/shared/api";
import { AccountsPanel } from "@/features/administration/AccountsPanel";
import { ChatViewerPanel } from "@/features/administration/ChatViewerPanel";
import { CodingTasksAdminPanel } from "@/features/administration/CodingTasksAdminPanel";
import { DeploymentsPanel } from "@/features/administration/DeploymentsPanel";
import { FaqPanel } from "@/features/administration/FaqPanel";
import { MergeRequestsPanel } from "@/features/administration/MergeRequestsPanel";
import { MihomoPanel } from "@/features/administration/MihomoPanel";
import { ModelKeyPanel } from "@/features/administration/ModelKeyPanel";
import { OverviewPanel } from "@/features/administration/OverviewPanel";
import { PlannerAdminPanel } from "@/features/administration/PlannerAdminPanel";
import { ErrorState, LoadingState } from "@/shared/ui/State";
import s from "./AdministrationPage.module.css";

type Tab =
	| "overview"
	| "accounts"
	| "chats"
	| "coding"
	| "faq"
	| "model"
	| "mihomo"
	| "planner"
	| "merges"
	| "deployments";

const tabs: { id: Tab; label: () => string }[] = [
	{ id: "overview", label: () => m.adminOverview() },
	{ id: "accounts", label: () => m.adminAccounts() },
	{ id: "chats", label: () => m.adminChats() },
	{ id: "coding", label: () => m.codingTasks() },
	{ id: "faq", label: () => m.adminFaq() },
	{ id: "model", label: () => m.adminModelKey() },
	{ id: "mihomo", label: () => m.adminMihomo() },
	{ id: "planner", label: () => m.plannerPageTitle() },
	{ id: "merges", label: () => m.adminMergeRequests() },
	{ id: "deployments", label: () => m.adminDeployments() },
];

export default function AdministrationPage() {
	const navigate = useNavigate();
	const [active, setActive] = createSignal<Tab>("overview");
	const [session, { refetch: retrySession }] = createResource(identity);
	const [dashboard, { refetch }] = createResource(
		() => {
			const current = session.error ? undefined : session();
			return current?.admin ? current : null;
		},
		async () => unwrap(await api.GET("/api/admin/dashboard")),
	);

	createEffect(() => {
		const current = session.error ? undefined : session();
		if ((session.error && isAuthError(session.error)) || current?.admin === false) {
			navigate("/admin/login", { replace: true });
		}
		if (dashboard.error && isAuthError(dashboard.error)) {
			navigate("/admin/login", { replace: true });
		}
	});

	return (
		<main class={s.page}>
			<div class={s.heading}>
				<div>
					<h1>{m.administrationPageTitle()}</h1>
					<p>{session.error ? undefined : session()?.username}</p>
				</div>
			</div>
			<Show when={!session.loading} fallback={<LoadingState />}>
				<Show
					when={!session.error && session()?.admin}
					fallback={<ErrorState onRetry={() => void retrySession()} />}
				>
					<nav class={s.tabs} aria-label={m.adminSections()}>
						<For each={tabs}>
							{(tab) => (
								<button
									type="button"
									data-active={active() === tab.id}
									aria-pressed={active() === tab.id}
									onClick={() => setActive(tab.id)}
								>
									{tab.label()}
								</button>
							)}
						</For>
					</nav>
					<Show when={active() === "overview"}>
						<Show
							when={!dashboard.loading || !!dashboard.latest}
							fallback={<LoadingState />}
						>
							<Show
								when={!dashboard.error && dashboard()}
								fallback={<ErrorState onRetry={() => void refetch()} />}
							>
								{(data) => (
									<OverviewPanel
										dashboard={data()}
										csrf={session()?.csrf}
										onChange={() => void refetch()}
									/>
								)}
							</Show>
						</Show>
					</Show>
					<Show when={active() === "accounts"}>
						<AccountsPanel csrf={session()?.csrf} />
					</Show>
					<Show when={active() === "chats"}>
						<ChatViewerPanel />
					</Show>
					<Show when={active() === "coding"}>
						<CodingTasksAdminPanel />
					</Show>
					<Show when={active() === "faq"}>
						<FaqPanel csrf={session()?.csrf} />
					</Show>
					<Show when={active() === "model"}>
						<ModelKeyPanel
							csrf={session()?.csrf}
							configured={session()?.modelReady}
						/>
					</Show>
					<Show when={active() === "mihomo"}>
						<MihomoPanel />
					</Show>
					<Show when={active() === "planner"}>
						<PlannerAdminPanel csrf={session()?.csrf} />
					</Show>
					<Show when={active() === "merges"}>
						<MergeRequestsPanel csrf={session()?.csrf} />
					</Show>
					<Show when={active() === "deployments"}>
						<DeploymentsPanel csrf={session()?.csrf} />
					</Show>
				</Show>
			</Show>
		</main>
	);
}

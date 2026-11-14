import { createSignal, For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { Markdown } from "@/shared/ui/Markdown";
import { EmptyState } from "@/shared/ui/State";
import { formatDate } from "@/shared/lib/format";
import type { components } from "@/shared/api/schema";
import s from "@/features/coding/CodingPage.module.css";

type CodingTaskDetails = components["schemas"]["CodingTaskDetails"];
type CodingMessage = components["schemas"]["CodingMessage"];
type Tab = "dev" | "read" | "changes" | "checks";
type ToolAudit = {
	operation?: string;
	input?: {
		path?: string;
		query?: string;
		check?: string;
		content?: string;
		plannerBlockId?: number;
	};
	output?: { ok?: boolean; content?: string; error?: string };
};

function parseToolAudit(message: CodingMessage): ToolAudit {
	try {
		return JSON.parse(message.content) as ToolAudit;
	} catch {
		return { operation: message.toolName };
	}
}

function toolTarget(audit: ToolAudit) {
	return (
		audit.input?.path ||
		audit.input?.query ||
		audit.input?.check ||
		(audit.input?.plannerBlockId ? `#${audit.input.plannerBlockId}` : "")
	);
}

export function AgentWorkTabs(props: { task: CodingTaskDetails }) {
	const [tab, setTab] = createSignal<Tab>("dev");
	const messages = () => props.task.messages ?? [];
	const regular = () => messages().filter((message) => message.role !== "tool");
	const tools = () => messages().filter((message) => message.role === "tool");
	const reading = () =>
		tools().filter((message) =>
			["list_files", "read_file", "search_code"].includes(message.toolName ?? ""),
		);
	const changes = () =>
		tools().filter((message) =>
			["write_file", "create_merge_request", "start_planner_block", "workspace"].includes(
				message.toolName ?? "",
			),
		);
	const checks = () => tools().filter((message) => message.toolName === "run_check");

	return (
		<>
			<nav class={s.detailTabs} aria-label={m.codingTabs()}>
				<button type="button" data-active={tab() === "dev"} aria-pressed={tab() === "dev"} onClick={() => setTab("dev")}>
					{m.codingTabDev()} <span>{regular().length}</span>
				</button>
				<button type="button" data-active={tab() === "read"} aria-pressed={tab() === "read"} onClick={() => setTab("read")}>
					{m.codingTabRead()} <span>{reading().length}</span>
				</button>
				<button type="button" data-active={tab() === "changes"} aria-pressed={tab() === "changes"} onClick={() => setTab("changes")}>
					{m.codingTabChanges()} <span>{changes().length}</span>
				</button>
				<button type="button" data-active={tab() === "checks"} aria-pressed={tab() === "checks"} onClick={() => setTab("checks")}>
					{m.codingTabChecks()} <span>{checks().length}</span>
				</button>
			</nav>

			<Show when={tab() === "dev"}>
				<section class={s.tabPanel} aria-live="polite">
					<For each={regular()} fallback={<EmptyState message={m.codingMessagesEmpty()} />}>
						{(message) => (
							<article class={s.message}>
								<strong>
									{message.role === "user" ? m.you() : m.assistant()}
								</strong>
								<Show
									when={message.role === "assistant"}
									fallback={<p>{message.content}</p>}
								>
									<Markdown content={message.content} />
								</Show>
								<time>{formatDate(message.createdAt)}</time>
							</article>
						)}
					</For>
				</section>
			</Show>

			<Show when={tab() === "read"}>
				<section class={s.tabPanel}>
					<For each={reading()} fallback={<EmptyState message={m.codingReadEmpty()} />}>
						{(message) => {
							const audit = parseToolAudit(message);
							return (
								<article class={s.fileCard}>
									<header class={s.fileHeader}>
										<strong>{audit.operation || message.toolName}</strong>
										<Show when={toolTarget(audit)}>
											<span class={s.filePath}>{toolTarget(audit)}</span>
										</Show>
									</header>
									<pre class={s.fileContent}>
										{audit.output?.content || message.content}
									</pre>
									<time>{formatDate(message.createdAt)}</time>
								</article>
							);
						}}
					</For>
				</section>
			</Show>

			<Show when={tab() === "changes"}>
				<section class={s.tabPanel}>
					<For each={changes()} fallback={<EmptyState message={m.codingChangesEmpty()} />}>
						{(message) => {
							const audit = parseToolAudit(message);
							return (
								<article class={s.fileCard}>
									<header class={s.fileHeader}>
										<strong>{audit.operation || message.toolName}</strong>
										<Show when={toolTarget(audit)}>
											<span class={s.filePath}>{toolTarget(audit)}</span>
										</Show>
									</header>
									<Show when={audit.input?.content}>
										<pre class={s.fileContent}>{audit.input?.content}</pre>
									</Show>
									<Show when={audit.output?.content}>
										<pre class={s.fileContent}>{audit.output?.content}</pre>
									</Show>
									<Show when={audit.output?.ok === false}>
										<p class={s.checkFailed} role="alert">
											{audit.output?.error || m.codingCheckFailed()}
										</p>
									</Show>
									<time>{formatDate(message.createdAt)}</time>
								</article>
							);
						}}
					</For>
					<section class={s.subsection}>
						<h3>{m.mergeRequestStatus()}</h3>
						<Show
							when={props.task.mergeRequestIid}
							fallback={<EmptyState message={m.mergeRequestEmpty()} />}
						>
							{(iid) => (
								<div>
									<Show
										when={props.task.mergeRequestUrl}
										fallback={<span>!{iid()}</span>}
									>
										{(url) => (
											<a href={url()} target="_blank" rel="noopener noreferrer">
												!{iid()}
											</a>
										)}
									</Show>
									<Show when={props.task.branch}>
										<p>{props.task.branch}</p>
									</Show>
								</div>
							)}
						</Show>
					</section>
				</section>
			</Show>

			<Show when={tab() === "checks"}>
				<section class={s.tabPanel}>
					<For each={checks()} fallback={<EmptyState message={m.codingChecksEmpty()} />}>
						{(message) => {
							const audit = parseToolAudit(message);
							return (
								<article class={s.fileCard}>
									<header class={s.fileHeader}>
										<strong>{audit.input?.check || message.toolName}</strong>
									</header>
									<p
										class={audit.output?.ok ? s.checkOk : s.checkFailed}
										role="status"
									>
										{audit.output?.ok ? m.codingCheckOk() : m.codingCheckFailed()}
									</p>
									<pre class={s.fileContent}>
										{audit.output?.content || audit.output?.error || message.content}
									</pre>
									<time>{formatDate(message.createdAt)}</time>
								</article>
							);
						}}
					</For>
				</section>
			</Show>
		</>
	);
}

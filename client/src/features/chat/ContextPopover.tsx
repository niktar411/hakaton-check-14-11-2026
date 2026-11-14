import { For, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import s from "./ChatPage.module.css";

export type ContextBreakdown = {
	userChars: number;
	assistantChars: number;
	draftChars: number;
	attachmentChars: number;
	total: number;
	top: { role: string; chars: number; preview: string }[];
	messageCount: number;
	remaining: number;
};

export type ContextAttachment = { id: string; name: string; text: string };

function formatNumber(value: number) {
	return value.toLocaleString("ru-RU");
}

export function ContextPopover(props: {
	breakdown: ContextBreakdown;
	attachments: ContextAttachment[];
	onMouseEnter: () => void;
	onMouseLeave: () => void;
}) {
	const share = (part: number) => {
		const total = props.breakdown.total;
		if (total <= 0) return 0;
		return Math.max(0, Math.min(100, (part / total) * 100));
	};
	return (
		<section
			id="chat-context-details"
			class={s.contextPopover}
			data-elevated
			aria-label={m.chatContextTitle()}
			onMouseEnter={props.onMouseEnter}
			onMouseLeave={props.onMouseLeave}
		>
			<strong>{m.chatContextTitle()}</strong>
			<div class={s.contextStack} aria-hidden="true">
				<span
					class={s.contextSegment}
					data-kind="user"
					style={{ width: `${share(props.breakdown.userChars)}%` }}
				/>
				<span
					class={s.contextSegment}
					data-kind="assistant"
					style={{ width: `${share(props.breakdown.assistantChars)}%` }}
				/>
				<span
					class={s.contextSegment}
					data-kind="attachments"
					style={{ width: `${share(props.breakdown.attachmentChars)}%` }}
				/>
				<span
					class={s.contextSegment}
					data-kind="draft"
					style={{ width: `${share(props.breakdown.draftChars)}%` }}
				/>
			</div>
			<dl class={s.contextRows}>
				<div>
					<dt>{m.chatContextUser()}</dt>
					<dd>
						{formatNumber(props.breakdown.userChars)} {m.chatContextCharsShort()}
					</dd>
				</div>
				<div>
					<dt>{m.chatContextAssistant()}</dt>
					<dd>
						{formatNumber(props.breakdown.assistantChars)} {m.chatContextCharsShort()}
					</dd>
				</div>
				<div>
					<dt>{m.chatContextAttachments()}</dt>
					<dd>
						{formatNumber(props.breakdown.attachmentChars)} {m.chatContextCharsShort()}
					</dd>
				</div>
				<div>
					<dt>{m.chatContextDraft()}</dt>
					<dd>
						{formatNumber(props.breakdown.draftChars)} {m.chatContextCharsShort()}
					</dd>
				</div>
			</dl>
			<Show when={props.attachments.length > 0}>
				<p class={s.contextSection}>{m.chatContextFiles()}</p>
				<ul class={s.contextFiles}>
					<For each={props.attachments}>
						{(file) => (
							<li>
								<span title={file.name}>{file.name}</span>
								<span>
									{formatNumber(file.text.length)} {m.chatContextCharsShort()}
								</span>
							</li>
						)}
					</For>
				</ul>
			</Show>
			<Show when={props.breakdown.top.length > 0}>
				<p class={s.contextSection}>{m.chatContextTop()}</p>
				<ul class={s.contextMessages}>
					<For each={props.breakdown.top}>
						{(item) => (
							<li>
								<span class={s.contextMessageRole}>
									{item.role === "user" ? m.you() : m.assistant()}
								</span>
								<span class={s.contextMessagePreview} title={item.preview}>
									{item.preview}
								</span>
								<span class={s.contextMessageChars}>{formatNumber(item.chars)}</span>
							</li>
						)}
					</For>
				</ul>
			</Show>
			<p class={s.contextLimits}>
				{m.chatContextMessages({ count: formatNumber(props.breakdown.messageCount) })}
			</p>
			<p class={s.contextLimits}>{m.chatContextLimits()}</p>
			<p class={s.contextFree}>
				{m.chatContextFree()}: {formatNumber(props.breakdown.remaining)}{" "}
				{m.chatContextCharsShort()}
			</p>
			<p class={s.contextNote}>{m.chatContextNote()}</p>
		</section>
	);
}

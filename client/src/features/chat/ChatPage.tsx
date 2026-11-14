import {
	createEffect,
	createMemo,
	createResource,
	createSignal,
	For,
	Index,
	onCleanup,
	onMount,
	Show,
} from "solid-js";
import * as m from "@/paraglide/messages";
import { api, ApiError, csrfHeaders, streamPost, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { LocalLoginForm } from "@/features/auth/LocalLoginForm";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { Markdown } from "@/shared/ui/Markdown";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import { AgentRunCard } from "./AgentRunCard";
import { ChatModeTabs } from "./ChatModeTabs";
import { ChatUpdateCard } from "./ChatUpdateCard";
import { ContextPopover, type ContextBreakdown } from "./ContextPopover";
import s from "./ChatPage.module.css";

type ChatSummary = components["schemas"]["ChatSummary"];
type ChatHistory = components["schemas"]["ChatHistory"];
type ChatNotification = components["schemas"]["ChatNotification"];
type Identity = components["schemas"]["Identity"];
type CodingTask = components["schemas"]["CodingTask"];
type CodingTaskDetails = components["schemas"]["CodingTaskDetails"];
type ChatUpdate = components["schemas"]["ChatUpdate"];

type Attachment = {
	id: string;
	name: string;
	size: number;
	text: string;
};

type StreamMessage = {
	role: "user" | "assistant";
	content: string;
	reasoning?: string;
	streaming?: boolean;
};

type StreamMeta = { id: number; phase: string };

type SpeechRecognitionResult = {
	isFinal: boolean;
	0: { transcript: string };
};

type SpeechRecognitionEvent = {
	resultIndex: number;
	results: ArrayLike<SpeechRecognitionResult>;
};

type SpeechRecognitionErrorEvent = {
	error: string;
};

type SpeechRecognitionInstance = {
	lang: string;
	continuous: boolean;
	interimResults: boolean;
	start: () => void;
	stop: () => void;
	onresult: ((event: SpeechRecognitionEvent) => void) | null;
	onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
	onend: (() => void) | null;
};

type SpeechRecognitionConstructor = new () => SpeechRecognitionInstance;

const MAX_FILES = 3;
const MAX_FILE_BYTES = 200 * 1024;
const MAX_PROMPT_CHARS = 16000;
const MAX_FILE_CHARS = 12000;
const MAX_ATTACHMENT_CHARS = 12000;
const COMPOSER_MAX_HEIGHT = 220;
const MIC_REQUESTED_KEY = "portal-mic-permission-requested";
const TEXT_FILE_PATTERN =
	/\.(?:txt|md|markdown|json|jsonl|csv|tsv|log|ya?ml|toml|ini|conf|xml|html?|css|scss|jsx?|tsx?|go|py|rs|java|kt|kts|sql|sh|bash|ps1|env|gitignore)$/i;

function isTextFile(file: File) {
	return (
		file.type.startsWith("text/") ||
		file.type === "application/json" ||
		TEXT_FILE_PATTERN.test(file.name)
	);
}

function speechRecognitionConstructor() {
	const target = window as Window & {
		SpeechRecognition?: SpeechRecognitionConstructor;
		webkitSpeechRecognition?: SpeechRecognitionConstructor;
	};
	return target.SpeechRecognition ?? target.webkitSpeechRecognition;
}

async function requestMicrophonePermission() {
	if (!navigator.mediaDevices?.getUserMedia) return;
	try {
		if (sessionStorage.getItem(MIC_REQUESTED_KEY)) return;
		const permission = navigator.permissions
			? await navigator.permissions.query({
					name: "microphone" as PermissionName,
				})
			: undefined;
		if (permission && permission.state !== "prompt") return;
		sessionStorage.setItem(MIC_REQUESTED_KEY, "1");
	} catch {
		// Some browsers do not expose the microphone permission descriptor.
		// The voice button still asks for access when the user starts dictation.
		return;
	}
	try {
		const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
		stream.getTracks().forEach((track) => {
			track.stop();
		});
	} catch {
		// A denial is reported by the voice button; do not spam a toast on open.
	}
}

export default function ChatPage() {
	const toast = useToast();
	const [text, setText] = createSignal("");
	const [selected, setSelected] = createSignal(0);
	const [pending, setPending] = createSignal(false);
	const [attachments, setAttachments] = createSignal<Attachment[]>([]);
	const [listening, setListening] = createSignal(false);
	const [voiceError, setVoiceError] = createSignal<string>();
	const [optimisticPrompt, setOptimisticPrompt] = createSignal("");
	const [liveAnswer, setLiveAnswer] = createSignal("");
	const [reasoningByConversation, setReasoningByConversation] = createSignal<
		Record<number, string>
	>({});
	const [contextPinned, setContextPinned] = createSignal(false);
	const [contextOpen, setContextOpen] = createSignal(false);
	const [followStream, setFollowStream] = createSignal(true);
	const [agentMode, setAgentMode] = createSignal(true);
	const [agentTask, setAgentTask] = createSignal<CodingTaskDetails>();
	const [agentPending, setAgentPending] = createSignal(false);
	let contextCloseTimer: number | undefined;
	let composer: HTMLTextAreaElement | undefined;
	let filePicker: HTMLInputElement | undefined;
	let recognition: SpeechRecognitionInstance | undefined;
	let streamController: AbortController | undefined;
	let messageList: HTMLDivElement | undefined;
	let messagesEnd: HTMLDivElement | undefined;
	let liveThinking: HTMLDivElement | undefined;

	const [session, { refetch: refetchSession }] = createResource(async () => {
		try {
			return unwrap(await api.GET("/api/me")) as Identity;
		} catch {
			return null;
		}
	});
	const [updates, { refetch: refetchUpdates }] = createResource(
		() => (session.loading ? undefined : session() ? "auth" : "anon"),
		async (mode): Promise<ChatUpdate[]> => {
			if (mode !== "auth") return [];
			return unwrap(await api.GET("/api/chat/updates")) ?? [];
		},
	);
	const [updateBusy, setUpdateBusy] = createSignal(false);
	const [conversations, { refetch: refetchConversations }] = createResource(
		() => (session.loading ? undefined : session() ? "auth" : "anon"),
		async (mode): Promise<ChatSummary[]> => {
			if (mode !== "auth") return [];
			return (
				unwrap(
					await api.GET("/api/chat/conversations", {
						params: { query: { limit: 50, offset: 0 } },
					}),
				) ?? []
			);
		},
	);
	const [history, { refetch: refetchHistory }] = createResource(
		() => (session() && selected() > 0 ? selected() : undefined),
		async (id): Promise<ChatHistory | undefined> => {
			if (!id) return undefined;
			return unwrap(
				await api.GET("/api/chat/conversations/{id}", {
					params: { path: { id } },
				}),
			);
		},
	);

	// Team events (completed blocks, created merge requests) arrive in the
	// chat as notifications. Unacknowledged ones stay visible until the
	// member confirms them.
	const [notifications, { refetch: refetchNotifications }] = createResource(
		() => (session.loading ? undefined : session() ? "auth" : "anon"),
		async (mode): Promise<ChatNotification[]> => {
			if (mode !== "auth") return [];
			return (
				unwrap(
					await api.GET("/api/chat/notifications", {
						params: { query: { limit: 10, offset: 0 } },
					}),
				) ?? []
			);
		},
	);
	const [dismissed, setDismissed] = createSignal<ReadonlySet<number>>(new Set());
	const visibleNotifications = createMemo(() =>
		(notifications() ?? []).filter(
			(notification) =>
				!notification.acknowledgedAt && !dismissed().has(notification.id),
		),
	);
	async function acknowledgeNotification(id: number) {
		setDismissed((previous) => new Set(previous).add(id));
		try {
			await api.POST("/api/chat/notifications/{id}/ack", {
				headers: csrfHeaders(session()?.csrf),
				params: { path: { id } },
			});
		} finally {
			refetchNotifications();
		}
	}

	// createResource keeps its previous value when the source becomes falsy,
	// so a new chat would still show the last conversation's messages. Only
	// trust history that belongs to the conversation selected right now.
	const currentHistory = createMemo(() => {
		const id = selected();
		const data = history();
		return id > 0 && data?.id === id ? data : undefined;
	});
	const messages = () => currentHistory()?.messages ?? [];
	const visibleConversations = createMemo(
		() =>
			conversations()?.filter((conversation) => conversation.messageCount > 0) ??
			[],
	);
	const current = createMemo(() =>
		conversations()?.find((conversation) => conversation.id === selected()),
	);
	const displayMessages = createMemo<StreamMessage[]>(() => {
		const list: StreamMessage[] = messages().map((message) => ({
			role: message.role === "user" ? "user" : "assistant",
			content: message.content,
		}));
		const reasoning = reasoningByConversation()[selected()] ?? "";
		const last = list[list.length - 1];
		if (reasoning && last?.role === "assistant") {
			last.reasoning = reasoning;
		}
		if (pending()) {
			list.push({ role: "user", content: optimisticPrompt() });
			list.push({
				role: "assistant",
				content: liveAnswer(),
				reasoning: reasoningByConversation()[selected()] ?? "",
				streaming: true,
			});
		}
		return list;
	});
	function addReasoning(id: number, text: string) {
		setReasoningByConversation((previous) => ({
			...previous,
			[id]: `${previous[id] ?? ""}${text}`,
		}));
	}
	function showContext() {
		window.clearTimeout(contextCloseTimer);
		setContextOpen(true);
	}
	function hideContext() {
		if (contextPinned()) return;
		contextCloseTimer = window.setTimeout(() => setContextOpen(false), 120);
	}
	function toggleContext() {
		if (contextPinned()) {
			setContextPinned(false);
			setContextOpen(false);
			return;
		}
		setContextPinned(true);
		setContextOpen(true);
	}

	async function sendAgent(prompt: string): Promise<boolean> {
		if (agentPending() || !session()) return false;
		setAgentPending(true);
		try {
			const created = unwrap(
				await api.POST("/api/chat/agent", {
					headers: csrfHeaders(session()?.csrf),
					body: { prompt },
				}),
			) as CodingTask;
			let details: CodingTaskDetails;
			try {
				details = unwrap(
					await api.GET("/api/coding/tasks/{id}", {
						params: { path: { id: created.id } },
					}),
				) as CodingTaskDetails;
			} catch {
				details = { ...created, messages: [] };
			}
			setAgentTask(details);
			return true;
		} catch (error) {
			toast.show(chatError(error));
			return false;
		} finally {
			setAgentPending(false);
		}
	}

	const attachmentText = () =>
		buildPrompt().length - text().trim().length;
	const contextChars = () => contextText().length;
	const contextTokens = () => Math.round(contextChars() / 4);
	const contextPercent = () => Math.min(100, Math.round(contextChars() / 1000));
	const contextBreakdown = createMemo<ContextBreakdown>(() => {
		const list = messages();
		const userChars = list
			.filter((message) => message.role === "user")
			.reduce((total, message) => total + message.content.length, 0);
		const assistantChars = list
			.filter((message) => message.role === "assistant")
			.reduce((total, message) => total + message.content.length, 0);
		const draftChars = text().trim().length;
		const attachmentChars = Math.max(
			0,
			buildPrompt().length - text().trim().length,
		);
		const total = userChars + assistantChars + draftChars + attachmentChars;
		const top = list
			.map((message) => ({
				role: message.role,
				chars: message.content.length,
				preview: message.content.replace(/\s+/g, " ").trim(),
			}))
			.sort((left, right) => right.chars - left.chars)
			.slice(0, 3);
		return {
			userChars,
			assistantChars,
			draftChars,
			attachmentChars,
			total,
			top,
			messageCount: list.length,
			remaining: Math.max(0, 100000 - total),
		};
	});
	const canConfirm = () =>
		currentHistory()?.phase === "clarifying" && messages().length >= 2;
	const canSend = () => buildPrompt().trim().length > 0;

	function contextText() {
		const parts = messages().map((message) => message.content);
		parts.push(buildPrompt());
		return parts.join("\n");
	}

	function buildPrompt() {
		const base = text().trim();
		const files = attachments().map(
			(file) => `${m.chatFileHeader({ name: file.name })}\n${file.text.trim()}`,
		);
		if (!files.length) return base;
		return [base, m.chatAttachmentsHeader(), ...files].filter(Boolean).join("\n\n");
	}

	function resizeComposer() {
		if (!composer) return;
		composer.style.height = "auto";
		const height = Math.min(composer.scrollHeight, COMPOSER_MAX_HEIGHT);
		composer.style.height = `${height}px`;
		composer.style.overflowY =
			composer.scrollHeight > COMPOSER_MAX_HEIGHT ? "auto" : "hidden";
	}

	createEffect(() => {
		// Read the signal so the textarea grows after every change, including
		// programmatic clears and voice transcription.
		text();
		queueMicrotask(resizeComposer);
	});

	onMount(() => {
		resizeComposer();
		void requestMicrophonePermission();
		// Team events are delivered to the chat asynchronously; poll for new
		// notifications while the page is open.
		const notificationsTimer = window.setInterval(() => {
			refetchNotifications();
		}, 30000);
		onCleanup(() => window.clearInterval(notificationsTimer));
	});
	onCleanup(() => {
		recognition?.stop();
		streamController?.abort();
		window.clearTimeout(contextCloseTimer);
	});

	createEffect(() => {
		// Follow the growing answer while the model is streaming, but stop as
		// soon as the reader scrolls up to inspect an earlier message.
		displayMessages();
		agentTask();
		if (!followStream() || !messageList) return;
		queueMicrotask(() => {
			if (!messageList) return;
			messageList.scrollTop = messageList.scrollHeight;
			// The reasoning body is its own scroll container; keep its tail in
			// view too so the stream reads continuously while it grows.
			if (liveThinking?.isConnected) {
				liveThinking.scrollTop = liveThinking.scrollHeight;
			}
		});
	});

	function onMessagesScroll() {
		if (!messageList) return;
		const distance =
			messageList.scrollHeight -
			messageList.scrollTop -
			messageList.clientHeight;
		setFollowStream(distance < 140);
	}

	createEffect(() => {
		const task = agentTask();
		if (task?.phase !== "running") return;
		const timer = window.setInterval(async () => {
			try {
				const updated = unwrap(
					await api.GET("/api/coding/tasks/{id}", {
						params: { path: { id: task.id } },
					}),
				) as CodingTaskDetails;
				setAgentTask(updated);
			} catch {
				// Keep the last state; the coding page remains the source of truth.
			}
		}, 4000);
		onCleanup(() => window.clearInterval(timer));
	});

	// Completed blocks arrive as chat prompts. Polling mirrors the planner
	// page cadence; the server deduplicates, so a repeat poll never duplicates.
	createEffect(() => {
		if (!session()) return;
		const timer = window.setInterval(() => void refetchUpdates(), 15000);
		onCleanup(() => window.clearInterval(timer));
	});

	async function pullUpdate(update: ChatUpdate) {
		if (updateBusy()) return;
		setUpdateBusy(true);
		try {
			unwrap(
				await api.POST("/api/chat/updates/{id}/pull", {
					headers: csrfHeaders(session()?.csrf),
					params: { path: { id: update.id } },
				}),
			);
			toast.show(m.chatUpdatePulled());
			await refetchUpdates();
			setAgentMode(false);
			await send(false, m.chatUpdatePrompt({
				title: update.title,
				owner: update.ownerName,
				branch: update.branch,
				url: update.mrUrl,
			}));
		} catch {
			toast.show(m.chatUpdatePullError());
		} finally {
			setUpdateBusy(false);
		}
	}

	async function addFiles(files: FileList | File[]) {
		const selectedFiles = Array.from(files);
		if (!selectedFiles.length) return;
		const accepted: Attachment[] = [];
		let remainingChars = MAX_ATTACHMENT_CHARS - attachmentText();
		for (const file of selectedFiles) {
			if (attachments().length + accepted.length >= MAX_FILES) {
				toast.show(m.chatFilesLimit());
				break;
			}
			if (file.size > MAX_FILE_BYTES) {
				toast.show(`${file.name}: ${m.chatFileTooLarge()}`);
				continue;
			}
			if (!isTextFile(file)) {
				toast.show(`${file.name}: ${m.chatFileUnsupported()}`);
				continue;
			}
			if (remainingChars <= 0) {
				toast.show(m.chatFilesLimit());
				break;
			}
			try {
				const content = (await file.text()).replaceAll("\u0000", "");
				const clipped = content.slice(
					0,
					Math.min(MAX_FILE_CHARS, remainingChars),
				);
				remainingChars -= clipped.length;
				accepted.push({
					id: crypto.randomUUID(),
					name: file.name,
					size: file.size,
					text: clipped,
				});
			} catch {
				toast.show(`${file.name}: ${m.chatFileReadError()}`);
			}
		}
		if (accepted.length) {
			setAttachments((currentAttachments) => [...currentAttachments, ...accepted]);
		}
	}

	function removeAttachment(id: string) {
		setAttachments((currentAttachments) =>
			currentAttachments.filter((attachment) => attachment.id !== id),
		);
	}

	async function toggleVoice() {
		if (listening()) {
			recognition?.stop();
			return;
		}
		setVoiceError(undefined);
		const Constructor = speechRecognitionConstructor();
		if (!Constructor) {
			toast.show(m.chatVoiceUnsupported());
			return;
		}
		try {
			const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
			stream.getTracks().forEach((track) => {
				track.stop();
			});
		} catch (error) {
			const message =
				error instanceof DOMException && error.name === "NotAllowedError"
					? m.chatVoicePermissionDenied()
					: m.chatVoiceNoMicrophone();
			setVoiceError(message);
			toast.show(message);
			return;
		}
		recognition = new Constructor();
		recognition.lang = "ru-RU";
		recognition.continuous = true;
		recognition.interimResults = true;
		let base = text();
		recognition.onresult = (event) => {
			let final = "";
			let interim = "";
			for (let index = event.resultIndex; index < event.results.length; index += 1) {
				const result = event.results[index];
				if (result.isFinal) final += result[0].transcript;
				else interim += result[0].transcript;
			}
			const separator = (value: string) => (value && !value.endsWith(" ") ? " " : "");
			if (final.trim()) {
				base = `${base}${separator(base)}${final.trim()}`;
			}
			const spoken = base + (interim.trim() ? `${separator(base)}${interim}` : "");
			setText(spoken);
		};
		recognition.onerror = (event) => {
			setListening(false);
			if (event.error === "aborted") return;
			let message = m.chatVoiceError();
			if (event.error === "not-allowed" || event.error === "service-not-allowed") {
				message = m.chatVoicePermissionDenied();
			} else if (event.error === "audio-capture") {
				message = m.chatVoiceNoMicrophone();
			} else if (event.error === "network") {
				message = m.chatVoiceNetworkError();
			}
			setVoiceError(message);
			toast.show(message);
		};
		recognition.onend = () => setListening(false);
		try {
			recognition.start();
			setListening(true);
		} catch {
			setListening(false);
			toast.show(m.chatVoiceError());
		}
	}

	async function send(confirm = false, override?: string) {
		const useDraft = !override?.trim();
		const prompt = override?.trim() || buildPrompt().trim() || (confirm ? m.confirmPrompt() : "");
		if (!prompt || pending() || agentPending() || !session()) return;
		if (Array.from(prompt).length > MAX_PROMPT_CHARS) {
			toast.show(m.chatPromptTooLong());
			return;
		}
		if (agentMode() && !confirm) {
			const agentDraftText = text();
			const agentDraftAttachments = attachments();
			if (useDraft) {
				setText("");
				setAttachments([]);
			}
			const started = await sendAgent(prompt);
			if (
				!started &&
				useDraft &&
				!text().trim() &&
				attachments().length === 0
			) {
				setText(agentDraftText);
				setAttachments(agentDraftAttachments);
			}
			return;
		}
		const draftText = text();
		const draftAttachments = attachments();
		streamController?.abort();
		const controller = new AbortController();
		streamController = controller;
		let conversationID = selected();
		let restoreDraft = false;
		setFollowStream(true);
		setPending(true);
		setOptimisticPrompt(prompt);
		setLiveAnswer("");
		setContextPinned(false);
		setContextOpen(false);
		if (useDraft) {
			// Clear the composer immediately so the sent text does not sit in the
			// field while the model streams. It is restored only after an error.
			setText("");
			setAttachments([]);
		}
		setReasoningByConversation((previous) => {
			const next = { ...previous };
			delete next[conversationID];
			return next;
		});
		try {
			await streamPost(
				"/api/chat/stream",
				{
					id: conversationID,
					prompt,
					confirm,
					requestId: crypto.randomUUID().replace(/-/g, ""),
				},
				csrfHeaders(session()?.csrf) ?? {},
				(event, data) => {
					if (event === "meta") {
						const meta = JSON.parse(data) as StreamMeta;
						if (meta.id > 0) {
							conversationID = meta.id;
							setSelected(meta.id);
						}
					} else if (event === "reasoning") {
						const payload = JSON.parse(data) as { text?: string };
						if (payload.text) addReasoning(conversationID, payload.text);
					} else if (event === "content") {
						const payload = JSON.parse(data) as { text?: string };
						const text = payload.text ?? "";
						if (text) setLiveAnswer((previous) => previous + text);
					} else if (event === "error") {
						const payload = JSON.parse(data) as { code?: string };
						throw new ApiError(502, payload.code || "model.unavailable");
					}
				},
				controller.signal,
			);
			await Promise.all([refetchHistory(), refetchConversations()]);
		} catch (error) {
			if (!controller.signal.aborted) {
				restoreDraft = true;
				toast.show(chatError(error));
			}
		} finally {
			if (
				restoreDraft &&
				useDraft &&
				!text().trim() &&
				attachments().length === 0
			) {
				setText(draftText);
				setAttachments(draftAttachments);
			}
			if (streamController === controller) streamController = undefined;
			setPending(false);
			setOptimisticPrompt("");
			setLiveAnswer("");
		}
	}

	function chatError(error: unknown) {
		if (!(error instanceof ApiError)) return m.loadError();
		switch (error.code) {
			case "model.not_configured":
				return m.modelUnavailable();
			case "model.access_denied":
				return m.modelAccessDenied();
			case "model.insufficient_funds":
				return m.modelInsufficientFunds();
			case "model.unavailable":
			case "model.empty_response":
				return m.modelError();
			case "chat.busy":
			case "chat.request_already_used":
				return m.chatBusy();
			case "coding_task.busy":
				return m.chatBusy();
			case "coding_task.access_denied":
			case "coding_task.invalid_input":
			case "coding_task.invalid_phase":
			case "coding_task.unavailable":
				return m.chatAgentError();
			case "chat.prompt_invalid":
				return m.chatPromptTooLong();
			case "chat.timeout":
				return m.chatTimeout();
			case "chat.start_new":
				return m.chatStartNew();
			case "chat.clarify_first":
				return m.chatClarifyFirst();
			case "auth.required":
				return m.loginFailed();
			default:
				return m.loadError();
		}
	}

	function newChat() {
		streamController?.abort();
		setPending(false);
		setOptimisticPrompt("");
		setLiveAnswer("");
		setContextPinned(false);
		setContextOpen(false);
		setFollowStream(true);
		setSelected(0);
		setText("");
		setAttachments([]);
	}

	return (
		<main class={s.page} data-mode={agentMode() ? "agent" : "chat"}>
			<Show
				when={!session.loading}
				fallback={
					<div class={s.auth}>
						<LoadingState />
					</div>
				}
			>
				<Show
					when={session()}
					fallback={
						<div class={s.auth}>
							<section class={s.authCard}>
								<h1>{m.teamLoginTitle()}</h1>
								<p>{m.codingLoginHint()}</p>
								<LocalLoginForm
									audience="coding"
									onSuccess={() => void refetchSession()}
								/>
							</section>
						</div>
					}
				>
					<aside class={s.sidebar}>
						<Button variant="primary" onClick={newChat}>
							{m.newChat()}
						</Button>
						<h2>{m.chatPast()}</h2>
						<Show
							when={visibleConversations().length > 0}
							fallback={<EmptyState message={m.chatHistoryEmpty()} />}
						>
							<For each={visibleConversations()}>
								{(conversation) => (
									<button
										type="button"
										class={s.conversation}
										data-selected={selected() === conversation.id}
										onClick={() => {
											streamController?.abort();
											setContextPinned(false);
											setContextOpen(false);
											setFollowStream(true);
											setSelected(conversation.id);
										}}
									>
										<strong>{conversation.title || m.chatUntitled()}</strong>
										<span>{formatDate(conversation.createdAt)}</span>
									</button>
								)}
							</For>
						</Show>
					</aside>
					<section class={s.chat}>
						<header class={s.chatHeader}>
							<div>
								<h1>{current()?.title || m.aiChatTitle()}</h1>
								<p>{m.chatHint()}</p>
							</div>
							<div class={s.context}>
								<button
									type="button"
									class={s.contextButton}
									aria-expanded={contextOpen() || contextPinned()}
									aria-controls="chat-context-details"
									onMouseEnter={showContext}
									onMouseLeave={hideContext}
									onFocus={showContext}
									onBlur={hideContext}
									onClick={toggleContext}
									onKeyDown={(event) => {
										if (event.key !== "Escape") return;
										setContextPinned(false);
										setContextOpen(false);
									}}
								>
									<span>
										{m.chatContext()}: ~{contextTokens()} · {contextPercent()}%
									</span>
								</button>
								<span
									class={s.contextBar}
									role="progressbar"
									aria-label={m.chatContext()}
									aria-valuemin={0}
									aria-valuemax={100}
									aria-valuenow={contextPercent()}
								>
									<span style={{ width: `${contextPercent()}%` }} />
								</span>
								<Show when={contextOpen() || contextPinned()}>
									<ContextPopover
										breakdown={contextBreakdown()}
										attachments={attachments()}
										onMouseEnter={showContext}
										onMouseLeave={hideContext}
									/>
								</Show>
							</div>
						</header>
						<ChatModeTabs agent={agentMode()} onChange={setAgentMode} />
						<Show when={session()?.modelReady === false}>
							<p class={s.modelWarning} role="status">
								{m.modelUnavailable()}
							</p>
						</Show>
						<ChatUpdateCard
							updates={updates() ?? []}
							busy={updateBusy()}
							onPull={(update) => void pullUpdate(update)}
						/>
						<Show when={visibleNotifications().length > 0}>
							<section class={s.notifications} aria-label={m.chatNotifications()}>
								<For each={visibleNotifications()}>
									{(notification) => (
										<article class={s.notification} data-kind={notification.kind}>
											<div class={s.notificationText}>
												<strong>{notification.title}</strong>
												<p>{notification.body}</p>
												<time>{formatDate(notification.createdAt)}</time>
											</div>
											<Button
												variant="secondary"
												onClick={() => void acknowledgeNotification(notification.id)}
											>
												{m.chatNotificationAck()}
											</Button>
										</article>
									)}
								</For>
							</section>
						</Show>
						<div
							class={s.messages}
							ref={messageList}
							role="log"
							aria-live="polite"
							aria-relevant="additions text"
							aria-busy={pending()}
							onScroll={onMessagesScroll}
						>
							<Show when={!agentMode()}>
							<Show
								when={
									selected() === 0 ||
									(!history.loading && !history.error) ||
									displayMessages().length > 0
								}
								fallback={
									history.error ? (
										<ErrorState onRetry={() => void refetchHistory()} />
									) : (
										<LoadingState />
									)
								}
							>
								<Show
									when={displayMessages().length > 0}
									fallback={<EmptyState message={m.chatEmpty()} />}
								>
								<Index each={displayMessages()}>
									{(message) => (
										<article
											class={s.bubble}
											data-role={message().role}
											data-streaming={message().streaming}
										>
											<strong>
												{message().role === "user" ? m.you() : m.assistant()}
											</strong>
											<Show
												when={
													message().reasoning ||
													(message().streaming && !message().content)
												}
											>
												<details
													class={s.thinking}
													data-live={message().streaming && !message().content}
													open={
														message().streaming && !message().content
															? true
															: undefined
													}
												>
													<summary class={s.thinkingSummary}>
														<span class={s.thinkingOrb} aria-hidden="true" />
														<span class={s.thinkingLabel}>
															{message().streaming && !message().content
																? m.chatThinking()
																: m.chatThought()}
														</span>
														<span class={s.thinkingChevron} aria-hidden="true" />
													</summary>
													<div
													class={s.thinkingBody}
													ref={(element) => {
														if (!(message().streaming && !message().content)) return;
														liveThinking = element;
													}}
												>
														<Show
															when={message().reasoning}
															fallback={
																<p class={s.thinkingWaiting}>
																	{m.chatThinkingWaiting()}
																</p>
															}
														>
															<p class={s.thinkingText}>
																{message().reasoning}
																<Show
																	when={
																		message().streaming && !message().content
																	}
																>
																	<span class={s.caret} aria-hidden="true" />
																</Show>
															</p>
														</Show>
													</div>
												</details>
											</Show>
											<Show when={message().content}>
												<Show
													when={message().role === "assistant"}
													fallback={<p>{message().content}</p>}
												>
													<Markdown content={message().content} />
												</Show>
												<Show when={message().streaming}>
													<span class={s.caret} aria-hidden="true" />
												</Show>
											</Show>
										</article>
									)}
								</Index>
								</Show>
							</Show>
							</Show>
							<Show when={agentMode()}>
								<Show when={agentTask()} fallback={<EmptyState message={m.chatAgentEmpty()} />}>
									{(task) => <AgentRunCard task={task()} />}
								</Show>
							</Show>
							<div ref={messagesEnd} class={s.messagesEnd} />
						</div>
						<form
							class={s.composer}
							onSubmit={(event) => {
								event.preventDefault();
								void send(false);
							}}
						>
							<Show when={voiceError()}>
								{(message) => (
									<div class={s.voiceError} role="alert">
										<span>{message()}</span>
										<Button type="button" onClick={() => void toggleVoice()}>
											{m.chatVoiceRetry()}
										</Button>
									</div>
								)}
							</Show>
							<Show when={attachments().length > 0}>
								<ul class={s.attachments} aria-label={m.chatAttachments()}>
									<For each={attachments()}>
										{(file) => (
											<li class={s.attachment}>
												<span title={file.name}>{file.name}</span>
												<button
													type="button"
													aria-label={`${m.delete()} ${file.name}`}
													onClick={() => removeAttachment(file.id)}
												>
													×
												</button>
											</li>
										)}
									</For>
								</ul>
								<p class={s.attachmentHint}>{m.chatAttachmentHint()}</p>
							</Show>
							<textarea
								ref={composer}
								required={attachments().length === 0}
								rows={1}
								maxlength={16000}
								value={text()}
								onInput={(event) => {
									setText(event.currentTarget.value);
									resizeComposer();
								}}
								onKeyDown={(event) => {
									if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
									event.preventDefault();
									void send(false);
								}}
								placeholder={m.chatPlaceholder()}
							/>
							<div class={s.composerActions}>
								<div class={s.composerTools}>
									<input
										ref={filePicker}
										class={s.filePicker}
										type="file"
										multiple
										accept=".txt,.md,.markdown,.json,.jsonl,.csv,.tsv,.log,.yaml,.yml,.toml,.ini,.conf,.xml,.html,.css,.scss,.js,.jsx,.ts,.tsx,.go,.py,.rs,.java,.kt,.sql,.sh,.ps1,text/*,application/json"
										onChange={(event) => {
											const files = event.currentTarget.files;
											if (files) void addFiles(files);
											event.currentTarget.value = "";
										}}
									/>
									<Button
										type="button"
										aria-label={m.chatAttach()}
										onClick={() => filePicker?.click()}
									>
										{m.chatAttach()}
									</Button>
									<Button
										type="button"
										aria-label={
											listening() ? m.chatVoiceStop() : m.chatVoice()
										}
										aria-pressed={listening()}
										data-listening={listening()}
										class={s.voiceButton}
										onClick={() => void toggleVoice()}
									>
										{listening() ? m.chatVoiceStop() : m.chatVoice()}
									</Button>
								</div>
								<div class={s.composerSend}>
									<Show when={!agentMode() && canConfirm()}>
										<Button
											type="button"
											disabled={pending()}
											onClick={() => void send(true, m.chatSkipPrompt())}
										>
											{m.chatSkipClarification()}
										</Button>
										<Button
											type="button"
											disabled={pending()}
											onClick={() => void send(true)}
										>
											{m.confirmSolution()}
										</Button>
									</Show>
									<Button
										type="submit"
										variant="primary"
										disabled={pending() || agentPending() || !canSend()}
									>
										{pending() || agentPending()
											? m.loading()
											: agentMode()
												? m.chatAgentSend()
												: m.send()}
									</Button>
								</div>
							</div>
						</form>
					</section>
				</Show>
			</Show>
		</main>
	);
}

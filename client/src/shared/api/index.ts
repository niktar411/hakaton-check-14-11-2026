import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

export const api = createClient<paths>();
export type ApiProblem = components["schemas"]["ErrorModel"];
export type ApiResult<T> = {
	data?: T;
	error?: ApiProblem;
	response: Response;
};

export class ApiError extends Error {
	constructor(
		readonly status: number,
		readonly code: string,
	) {
		super(code);
		this.name = "ApiError";
	}
}

export function unwrap<T>(result: ApiResult<T>): T {
	if (result.data === undefined) {
		if (result.response.ok) return result.data as T;
		throw new ApiError(
			result.response.status,
			result.error?.detail ?? "request.failed",
		);
	}
	return result.data;
}

export function isAuthError(error: unknown) {
	return (
		error instanceof ApiError &&
		(error.status === 401 || error.status === 403)
	);
}

export async function identity() {
	return unwrap(await api.GET("/api/me"));
}

export function requestId() {
	return Array.from(crypto.getRandomValues(new Uint8Array(24)), (value) =>
		value.toString(16).padStart(2, "0"),
	).join("");
}

export function csrfHeaders(csrf?: string) {
	return csrf ? { "X-CSRF-Token": csrf } : undefined;
}

export function downloadText(filename: string, content: string, type: string) {
	const url = URL.createObjectURL(new Blob([content], { type }));
	const anchor = document.createElement("a");
	anchor.href = url;
	anchor.download = filename;
	anchor.click();
	setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function downloadProfile(id: string, configuration: string) {
	downloadText(`${id}.conf`, configuration, "text/plain");
}

async function readEventStream(
	body: ReadableStream<Uint8Array>,
	onEvent: (event: string, data: string) => void,
) {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	const boundaryPattern = /\r?\n\r?\n/;
	let buffer = "";
	const dispatch = (frame: string) => {
		let event = "message";
		const data: string[] = [];
		for (const line of frame.split(/\r?\n/)) {
			if (line.startsWith("event:")) event = line.slice(6).trim();
			else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
		}
		if (data.length) onEvent(event, data.join("\n"));
	};
	try {
		for (;;) {
			const { value, done } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			let boundary = boundaryPattern.exec(buffer);
			while (boundary) {
				dispatch(buffer.slice(0, boundary.index));
				buffer = buffer.slice(boundary.index + boundary[0].length);
				boundary = boundaryPattern.exec(buffer);
			}
		}
		buffer += decoder.decode();
		if (buffer.trim()) dispatch(buffer);
	} finally {
		try {
			await reader.cancel();
		} catch {
			// The stream is already closed or aborted.
		}
		reader.releaseLock();
	}
}

// streamPost sends a JSON body and consumes a Server-Sent Events response.
// The generated OpenAPI client cannot stream, so this is the one place that
// talks to fetch directly.
export async function streamPost(
	path: string,
	payload: unknown,
	headers: Record<string, string>,
	onEvent: (event: string, data: string) => void,
	signal?: AbortSignal,
) {
	const controller = new AbortController();
	const timeout = window.setTimeout(() => controller.abort(), 180_000);
	const forwardAbort = () => controller.abort();
	if (signal) {
		if (signal.aborted) controller.abort();
		else signal.addEventListener("abort", forwardAbort, { once: true });
	}
	try {
		const response = await fetch(path, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				Accept: "text/event-stream",
				...headers,
			},
			body: JSON.stringify(payload),
			signal: controller.signal,
		});
		if (!response.ok) {
			let code = "request.failed";
			try {
				const problem = (await response.json()) as {
					code?: string;
					detail?: string;
				};
				if (problem.code) code = problem.code;
				else if (problem.detail) code = problem.detail;
			} catch {
				// The connection can close without a JSON problem body.
			}
			throw new ApiError(response.status, code);
		}
		if (!response.body) throw new ApiError(502, "model.unavailable");
		await readEventStream(response.body, onEvent);
	} catch (error) {
		if (controller.signal.aborted && !signal?.aborted) {
			throw new ApiError(504, "chat.timeout");
		}
		throw error;
	} finally {
		window.clearTimeout(timeout);
		signal?.removeEventListener("abort", forwardAbort);
	}
}

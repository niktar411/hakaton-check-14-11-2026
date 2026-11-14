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

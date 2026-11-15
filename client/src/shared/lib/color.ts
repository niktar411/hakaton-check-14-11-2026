const colorPattern = /^#[0-9a-f]{6}$/i;

/**
 * Uses only the server-validated owner color. Anything else falls back to the
 * theme border token so a malformed value can never inject extra CSS.
 */
export function safeColor(value: string | undefined) {
	return value && colorPattern.test(value) ? value : "var(--border)";
}

export function formatDate(value: string) {
	return new Intl.DateTimeFormat("ru-RU", {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(new Date(value));
}

export function formatBytes(value: number) {
	return `${(value / 1024 / 1024 / 1024).toFixed(1)} GiB`;
}

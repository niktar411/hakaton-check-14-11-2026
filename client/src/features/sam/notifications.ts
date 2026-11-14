// Дедупликация уведомлений SAM: ключ риска + локальное хранение прочитанных.
// Модуль не содержит UI и не отправляет события на сервер.

import { createSignal } from "solid-js";

const STORAGE_KEY = "hackathon-sam-notifications-seen";
const LIMIT = 200;

// Общий счётчик непрочитанных критических рисков: пишет SamNotifications,
// читает колокольчик в каркасе раздела. Модульный сигнал — как в Toast.
const [unseenCount, setUnseenCount] = createSignal(0);

export { setUnseenCount, unseenCount };

export function notificationKey(finding: {
	code: string;
	product?: string;
	asset?: string;
}): string {
	return [finding.code, finding.product ?? "", finding.asset ?? ""].join("\u0000");
}

export function readSeen(): string[] {
	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (raw === null) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.filter((value): value is string => typeof value === "string");
	} catch {
		return [];
	}
}

export function writeSeen(keys: string[]): void {
	const unique = [...new Set(keys)].slice(-LIMIT);
	try {
		window.localStorage.setItem(STORAGE_KEY, JSON.stringify(unique));
	} catch {
		return;
	}
	window.dispatchEvent(new CustomEvent("sam-notifications-changed"));
}

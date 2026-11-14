// Хранилище согласия на обработку данных и аналитику.
// Модуль не содержит UI: его читает баннер и код, которому важно,
// загружать ли аналитику до явного выбора пользователя.

export type Consent = {
	necessary: true;
	analytics: boolean;
	updatedAt: string;
};

const STORAGE_KEY = "hackathon-consent";

function isConsent(value: unknown): value is Consent {
	if (typeof value !== "object" || value === null) return false;
	const record = value as Record<string, unknown>;
	return (
		record.necessary === true &&
		typeof record.analytics === "boolean" &&
		typeof record.updatedAt === "string"
	);
}

export function readConsent(): Consent | null {
	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (raw === null) return null;
		const parsed: unknown = JSON.parse(raw);
		return isConsent(parsed) ? parsed : null;
	} catch {
		return null;
	}
}

export function writeConsent(analytics: boolean): Consent {
	const value: Consent = {
		necessary: true,
		analytics,
		updatedAt: new Date().toISOString(),
	};
	try {
		window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
	} catch {
		return value;
	}
	window.dispatchEvent(new CustomEvent("consent-changed", { detail: value }));
	return value;
}

export function analyticsAllowed(): boolean {
	return readConsent()?.analytics === true;
}

export function formatDate(value: string) {
	return new Intl.DateTimeFormat("ru-RU", {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(new Date(value));
}

export function formatDay(value: string) {
	return new Intl.DateTimeFormat("ru-RU", {
		dateStyle: "medium",
	}).format(new Date(value));
}

export function formatBytes(value: number) {
	return `${(value / 1024 / 1024 / 1024).toFixed(1)} GiB`;
}

const moneyFormat = new Intl.NumberFormat("ru-RU", {
	style: "currency",
	currency: "RUB",
	maximumFractionDigits: 0,
});

const numberFormat = new Intl.NumberFormat("ru-RU");

export function formatMoney(value: number) {
	return moneyFormat.format(value);
}

export function formatNumber(value: number) {
	return numberFormat.format(value);
}

import { expect, type Page, test } from "@playwright/test";

type Mode = "loading" | "empty" | "error" | "success";

const summary = {
	products: 5,
	activeProducts: 4,
	bannedProducts: 1,
	assets: 12,
	licenses: 5,
	installations: 146,
	devices: 146,
	unmatchedInstallations: 3,
	purchasedSeats: 155,
	licensedSeats: 155,
	expiredSeats: 10,
	overusedSeats: 18,
	idleSeats: 40,
	bannedInstallations: 8,
	annualCost: 125000,
	overuseCost: 18000,
	idleCost: 20000,
	expiredCost: 8000,
	costAtRisk: 26000,
	compliancePercent: 60,
	riskLevel: "high",
	recommendations: [
		"Удалить запрещённое ПО с 8 устройств.",
		"Докупить 18 мест или удалить лишние установки.",
		"Сопоставить 3 установок с каталогом.",
	],
	categories: [
		{
			category: "Office",
			products: 2,
			installedDevices: 128,
			overusedSeats: 18,
			riskCost: 18000,
		},
	],
	statuses: [
		{ status: "active", products: 4, installedDevices: 140 },
		{ status: "inactive", products: 0, installedDevices: 0 },
		{ status: "archived", products: 0, installedDevices: 0 },
		{ status: "banned", products: 1, installedDevices: 8 },
	],
};

const emptySummary = {
	...summary,
	products: 0,
	activeProducts: 0,
	bannedProducts: 0,
	assets: 0,
	licenses: 0,
	installations: 0,
	devices: 0,
	unmatchedInstallations: 0,
	purchasedSeats: 0,
	licensedSeats: 0,
	expiredSeats: 0,
	overusedSeats: 0,
	idleSeats: 0,
	bannedInstallations: 0,
	annualCost: 0,
	overuseCost: 0,
	idleCost: 0,
	expiredCost: 0,
	costAtRisk: 0,
	compliancePercent: 0,
	riskLevel: "low",
	recommendations: ["Критических действий не требуется."],
	categories: [],
	statuses: [],
};

const products = [
	{
		id: 1,
		name: "Microsoft Office",
		vendor: "Microsoft",
		category: "Office",
		status: "active",
		licenses: 1,
		purchasedSeats: 100,
		licensedSeats: 100,
		expiredSeats: 0,
		installedDevices: 118,
		installations: 120,
		bannedInstallations: 0,
		overusedSeats: 18,
		idleSeats: 0,
		deltaSeats: 18,
		annualCost: 100000,
		avgCostPerSeat: 1000,
		riskCost: 18000,
		riskLevel: "high",
		recommendation: "Докупить 18 мест или удалить лишние установки.",
	},
	{
		id: 4,
		name: "Radmin",
		vendor: "Famatech",
		category: "Remote access",
		status: "banned",
		licenses: 0,
		purchasedSeats: 0,
		licensedSeats: 0,
		expiredSeats: 0,
		installedDevices: 8,
		installations: 8,
		bannedInstallations: 8,
		overusedSeats: 0,
		idleSeats: 0,
		deltaSeats: 8,
		annualCost: 0,
		avgCostPerSeat: 0,
		riskCost: 0,
		riskLevel: "high",
		recommendation: "Удалить запрещённое ПО с 8 устройств.",
	},
];

const risks = [
	{
		code: "license_overuse",
		severity: "high",
		productId: 1,
		productName: "Microsoft Office",
		category: "Office",
		affected: 18,
		cost: 18000,
		recommendation: "Докупить 18 мест или удалить лишние установки.",
	},
	{
		code: "unmatched_installations",
		severity: "high",
		affected: 3,
		cost: 0,
		recommendation: "Сопоставить 3 установок с каталогом.",
	},
];

const productDetail = {
	product: products[0],
	licenses: [
		{
			id: 1,
			kind: "subscription",
			seatsTotal: 100,
			seatsUsed: 100,
			costPerSeat: 1000,
			currency: "RUB",
			validUntil: "2027-01-15",
			status: "active",
		},
	],
	hosts: [
		{
			assetId: 1,
			hostname: "alpha-linux",
			platform: "Linux",
			owner: "Леонид",
			version: "16.89",
			lastSeenAt: "2026-11-14T09:30:00Z",
			licenseStatus: "licensed",
		},
	],
};

const hostDetail = {
	id: 1,
	hostname: "alpha-linux",
	platform: "Linux",
	owner: "Леонид",
	lastSeenAt: "2026-11-14T09:30:00Z",
	installations: [
		{
			id: 1,
			productId: 1,
			product: "Microsoft Office",
			vendor: "Microsoft",
			category: "Office",
			detectedName: "Microsoft 365",
			version: "16.89",
			status: "licensed",
		},
	],
	licensed: 1,
	expired: 0,
	unlicensed: 0,
	banned: 0,
	inactive: 0,
};

async function mockBase(page: Page) {
	await page.route("**/api/public/site-config", (route) =>
		route.fulfill({
			json: {
				publicBaseUrl: "https://portal.test/",
				plannerBaseUrl: "https://planner.test/team",
				links: [],
			},
		}),
	);
	await page.route("**/api/me", (route) =>
		route.fulfill({ status: 401, json: { detail: "auth.required" } }),
	);
}

async function mockCompliance(page: Page) {
	await mockBase(page);
	const state = { mode: "success" as Mode };
	await page.route("**/api/public/sam/compliance/**", async (route) => {
		const path = new URL(route.request().url()).pathname;
		if (state.mode === "loading") {
			await new Promise((resolve) => setTimeout(resolve, 250));
		}
		if (state.mode === "error") {
			return route.fulfill({
				status: 503,
				json: { detail: "sam.storage_unavailable" },
			});
		}
		if (state.mode === "empty") {
			if (path.endsWith("/summary")) return route.fulfill({ json: emptySummary });
			return route.fulfill({ json: [] });
		}
		if (path.endsWith("/summary")) return route.fulfill({ json: summary });
		if (path.endsWith("/products/1")) {
			return route.fulfill({ json: productDetail });
		}
		if (path.endsWith("/hosts/1")) return route.fulfill({ json: hostDetail });
		if (path.endsWith("/products")) return route.fulfill({ json: products });
		if (path.endsWith("/risks")) return route.fulfill({ json: risks });
		return route.fulfill({ status: 404, json: { detail: "not_found" } });
	});
	return state;
}

type SamCase = {
	path: string;
	api: string;
	heading: string;
	empty: unknown;
	success: unknown;
	emptyText: string;
	successText: string;
};

const cases: SamCase[] = [
	{
		path: "/sam/catalog",
		api: "/api/public/sam/catalog",
		heading: "SAM: каталог",
		empty: [],
		success: [
			{
				id: 7,
				name: "Design Suite",
				vendor: "Acme",
				category: "Design",
				licenseCount: 2,
				installationCount: 5,
				updatedAt: "2026-11-14T12:00:00Z",
			},
		],
		emptyText: "В каталоге пока нет продуктов",
		successText: "Design Suite",
	},
	{
		path: "/sam/licenses",
		api: "/api/public/sam/licenses",
		heading: "SAM: лицензии",
		empty: [],
		success: [
			{
				id: 1,
				product: "Design Suite",
				vendor: "Acme",
				kind: "subscription",
				seatsTotal: 10,
				seatsUsed: 4,
				validUntil: "2027-01-15",
				status: "active",
			},
		],
		emptyText: "Лицензий пока нет",
		successText: "Активна",
	},
	{
		path: "/sam/inventory",
		api: "/api/public/sam/inventory",
		heading: "SAM: инвентаризация",
		empty: [],
		success: [
			{
				id: 1,
				hostname: "alpha",
				platform: "Linux",
				owner: "team",
				softwareCount: 8,
				lastSeenAt: "2026-11-14T09:30:00Z",
			},
		],
		emptyText: "Устройств пока нет",
		successText: "alpha",
	},
	{
		path: "/sam/matching",
		api: "/api/public/sam/matching",
		heading: "SAM: сопоставление",
		empty: [],
		success: [
			{
				id: 4,
				detectedName: "design-suite",
				version: "12.1",
				asset: "alpha",
				product: "Design Suite",
				confidence: 0.92,
				status: "suggested",
				createdAt: "2026-11-14T10:00:00Z",
			},
		],
		emptyText: "Предложений пока нет",
		successText: "92%",
	},
	{
		path: "/sam/assistant",
		api: "/api/public/sam/assistant",
		heading: "SAM: ассистент",
		empty: { suggestions: [] },
		success: {
			suggestions: [
				{ kind: "expiring_licenses", count: 2, path: "/sam/licenses" },
			],
		},
		emptyText: "Рекомендаций пока нет",
		successText: "Истекают лицензии",
	},
];

async function mockSam(page: Page) {
	await mockBase(page);
	const state = { mode: "success" as Mode };
	await page.route("**/api/public/sam/**", async (route) => {
		const path = new URL(route.request().url()).pathname;
		if (path.startsWith("/api/public/sam/compliance")) {
			return route.fallback();
		}
		const item = cases.find((candidate) => candidate.api === path);
		if (!item) return route.fulfill({ status: 404, json: { detail: "not_found" } });
		if (state.mode === "loading") {
			await new Promise((resolve) => setTimeout(resolve, 250));
		}
		if (state.mode === "error") {
			return route.fulfill({
				status: 503,
				json: { detail: "sam.storage_unavailable" },
			});
		}
		return route.fulfill({
			json: state.mode === "empty" ? item.empty : item.success,
		});
	});
	return state;
}

test("SAM opens from the public header navigation", async ({ page }) => {
	await mockCompliance(page);
	await page.goto("/");
	await page.getByRole("link", { name: "SAM", exact: true }).click();
	await expect(page).toHaveURL(/\/sam$/);
	await expect(page.getByRole("heading", { name: "SAM: обзор" })).toBeVisible();
	await expect(page.getByRole("navigation", { name: "Разделы SAM" })).toBeVisible();
});

test("dashboard exposes loading, empty, error and success", async ({ page }) => {
	const state = await mockCompliance(page);

	state.mode = "loading";
	await page.goto("/sam");
	await expect(page.getByRole("heading", { name: "SAM: обзор" })).toBeVisible();
	await expect(page.locator('[aria-busy="true"]')).toBeVisible();
	await expect(page.getByText("Комплаенс", { exact: false }).first()).toBeVisible();

	state.mode = "empty";
	await page.goto("/sam");
	await expect(page.getByText("Показателей пока нет", { exact: true })).toBeVisible();

	state.mode = "error";
	await page.goto("/sam");
	await expect(page.getByRole("alert")).toBeVisible();
	await expect(
		page.getByRole("button", { name: "Обновить", exact: true }),
	).toBeVisible();

	state.mode = "success";
	await page.goto("/sam");
	await expect(page.getByText("Microsoft Office", { exact: false }).first()).toBeVisible();
});

test("compliance table exposes loading, empty, error and success", async ({
	page,
}) => {
	const state = await mockCompliance(page);

	state.mode = "loading";
	await page.goto("/sam/compliance");
	await expect(page.getByRole("heading", { name: "SAM: комплаенс" })).toBeVisible();
	await expect(page.locator('[aria-busy="true"]')).toBeVisible();
	await expect(page.getByText("Microsoft Office", { exact: false }).first()).toBeVisible();

	state.mode = "empty";
	await page.goto("/sam/compliance");
	await expect(page.getByText("Нарушений не найдено", { exact: true })).toBeVisible();

	state.mode = "error";
	await page.goto("/sam/compliance");
	await expect(page.getByRole("alert")).toBeVisible();

	state.mode = "success";
	await page.goto("/sam/compliance");
	await expect(page.getByText("Microsoft Office", { exact: false }).first()).toBeVisible();
	await expect(page.getByText("Куплено", { exact: true }).first()).toBeVisible();
});

test("risk rows lead to product and host details", async ({ page }) => {
	await mockCompliance(page);
	await page.goto("/sam/compliance");
	await page.getByRole("link", { name: "Microsoft Office", exact: true }).click();
	await expect(page).toHaveURL(/\/sam\/catalog\/1$/);
	await expect(
		page.getByRole("heading", { name: "Microsoft Office" }),
	).toBeVisible();
	await page.getByRole("link", { name: "alpha-linux", exact: true }).click();
	await expect(page).toHaveURL(/\/sam\/inventory\/1$/);
	await expect(
		page.getByRole("heading", { name: "alpha-linux" }),
	).toBeVisible();
});

test("theme toggle switches the whole site and persists", async ({ page }) => {
	await page.emulateMedia({ colorScheme: "light" });
	const state = await mockCompliance(page);
	state.mode = "success";
	await page.goto("/");
	const toggle = page.getByRole("button", { name: "Переключить тему" });
	await expect(toggle).toBeVisible();
	await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

	await toggle.click();
	await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
	await expect(toggle).toHaveText("Светлая тема");

	await page.goto("/sam");
	await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
	await page.reload();
	await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");

	await toggle.click();
	await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
	await page.goto("/");
	await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});

test("every SAM page exposes loading, empty, error and success", async ({
	page,
}) => {
	const state = await mockSam(page);
	for (const item of cases) {
		state.mode = "loading";
		await page.goto(item.path);
		await expect(page.getByRole("heading", { name: item.heading })).toBeVisible();
		await expect(page.locator('[aria-busy="true"]')).toBeVisible();
		await expect(page.getByText(item.successText, { exact: false }).first()).toBeVisible();

		state.mode = "empty";
		await page.goto(item.path);
		await expect(page.getByText(item.emptyText, { exact: true })).toBeVisible();

		state.mode = "error";
		await page.goto(item.path);
		await expect(page.getByRole("alert")).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Обновить", exact: true }),
		).toBeVisible();

		state.mode = "success";
		await page.goto(item.path);
		await expect(page.getByText(item.successText, { exact: false }).first()).toBeVisible();
	}
});

test("SAM pages fit 375px and 1280px without horizontal scrolling", async ({
	page,
}) => {
	const compliance = await mockCompliance(page);
	compliance.mode = "success";
	const sam = await mockSam(page);
	sam.mode = "success";
	const paths = [
		"/sam",
		"/sam/compliance",
		"/sam/catalog",
		"/sam/catalog/1",
		"/sam/licenses",
		"/sam/inventory",
		"/sam/inventory/1",
		"/sam/matching",
		"/sam/assistant",
	];
	await page.addInitScript(() => {
		localStorage.setItem("portal-theme", "dark");
	});
	for (const width of [375, 1280]) {
		await page.setViewportSize({ width, height: 900 });
		for (const path of paths) {
			await page.goto(path);
			await expect(
				page.getByRole("navigation", { name: "Разделы SAM" }),
			).toBeVisible();
			await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
			expect(
				await page.evaluate(
					() =>
						document.documentElement.scrollWidth <=
						document.documentElement.clientWidth,
				),
			).toBe(true);
		}
	}
});

import { expect, type Page, test } from "@playwright/test";

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
		path: "/sam",
		api: "/api/public/sam/dashboard",
		heading: "SAM: обзор",
		empty: {
			products: 0,
			assets: 0,
			licenses: 0,
			seatsTotal: 0,
			seatsUsed: 0,
			openFindings: 0,
			pendingMatches: 0,
			expiringLicenses: 0,
			unlicensedInstallations: 0,
		},
		success: {
			products: 4,
			assets: 5,
			licenses: 6,
			seatsTotal: 70,
			seatsUsed: 42,
			openFindings: 3,
			pendingMatches: 2,
			expiringLicenses: 1,
			unlicensedInstallations: 7,
		},
		emptyText: "Показателей пока нет",
		successText: "Продукты",
	},
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
		path: "/sam/compliance",
		api: "/api/public/sam/compliance",
		heading: "SAM: комплаенс",
		empty: [],
		success: [
			{
				id: 3,
				code: "unlicensed-software",
				severity: "critical",
				status: "open",
				product: "Design Suite",
				asset: "alpha",
				details: "No active license",
				detectedAt: "2026-11-14T10:00:00Z",
			},
		],
		emptyText: "Нарушений не найдено",
		successText: "Критическая",
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

type Mode = "loading" | "empty" | "error" | "success";

async function mockSam(page: Page) {
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
	const state = { mode: "success" as Mode };
	await page.route("**/api/public/sam/**", async (route) => {
		const path = new URL(route.request().url()).pathname;
		const item = cases.find((candidate) => candidate.api === path);
		if (!item) return route.continue();
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
	const state = await mockSam(page);
	state.mode = "success";
	await page.goto("/");
	await page.getByRole("link", { name: "SAM", exact: true }).click();
	await expect(page).toHaveURL(/\/sam$/);
	await expect(page.getByRole("heading", { name: "SAM: обзор" })).toBeVisible();
	await expect(page.getByRole("navigation", { name: "Разделы SAM" })).toBeVisible();
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
	const state = await mockSam(page);
	state.mode = "success";
	for (const width of [375, 1280]) {
		await page.setViewportSize({ width, height: 900 });
		for (const item of cases) {
			await page.goto(item.path);
			await expect(
				page.getByRole("heading", { name: item.heading }),
			).toBeVisible();
			await expect(
				page.getByRole("navigation", { name: "Разделы SAM" }),
			).toBeVisible();
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

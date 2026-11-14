import { expect, type Page, test } from "@playwright/test";

async function publicConfig(page: Page) {
	await page.route("**/api/public/site-config", (route) =>
		route.fulfill({
			json: {
				publicBaseUrl: "https://portal.test/",
				plannerBaseUrl: "https://planner.test/team",
				links: [],
			},
		}),
	);
}

async function adminSession(page: Page) {
	await page.route("**/api/me", (route) =>
		route.fulfill({
			json: {
				username: "admin",
				admin: true,
				csrf: "admin-csrf",
				modelReady: true,
			},
		}),
	);
	await page.route("**/api/admin/dashboard", (route) =>
		route.fulfill({
			json: {
				servers: [],
				peers: [],
				totals: [],
				audit: [],
				containers: [],
			},
		}),
	);
	await page.route("**/api/admin/usage?*", (route) => route.fulfill({ json: [] }));
	await page.route("**/api/admin/accounts?*", (route) =>
		route.fulfill({
			json: [
				{
					id: 1,
					username: "admin",
					displayName: "Администратор",
					role: "admin",
					enabled: true,
					color: "#654321",
					personalContext: "",
					tone: "",
					createdAt: "2026-11-01T10:00:00Z",
					updatedAt: "2026-11-01T10:00:00Z",
				},
			],
		}),
	);
	await page.route("**/api/admin/coding/tasks?*", (route) =>
		route.fulfill({ json: [] }),
	);
	await page.route("**/api/admin/questions/context", (route) =>
		route.fulfill({ json: { context: "" } }),
	);
	await page.route("**/api/admin/questions/history?*", (route) =>
		route.fulfill({ json: { items: [], limit: 50, offset: 0 } }),
	);
	await page.route("**/api/public/planner", (route) => route.fulfill({ json: [] }));
	await page.route("**/api/admin/mihomo", (route) =>
		route.fulfill({ json: { configuration: "" } }),
	);
	await page.route("**/api/admin/deployments?*", (route) =>
		route.fulfill({ json: [] }),
	);
}

test("public pages stay indexable while admin and coding are noindex", async ({
	page,
}) => {
	await publicConfig(page);
	await page.route("**/api/coding/tasks", (route) =>
		route.fulfill({ status: 401, json: { detail: "auth.required" } }),
	);
	await page.route("**/api/me", (route) =>
		route.fulfill({ status: 401, json: { detail: "auth.required" } }),
	);
	await page.goto("/");
	await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
		"content",
		"index,follow",
	);
	await page.goto("/coding");
	await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
		"content",
		"noindex,nofollow",
	);
	await page.goto("/admin");
	await expect(page.getByRole("heading", { name: "Вход администратора" })).toBeVisible();
	await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
		"content",
		"noindex,nofollow",
	);
});

test("every administration section renders real data states", async ({ page }) => {
	const errors: Error[] = [];
	page.on("pageerror", (error) => errors.push(error));
	await publicConfig(page);
	await adminSession(page);
	await page.goto("/admin");

	await expect(page.getByRole("heading", { name: "Управление командой" })).toBeVisible();

	await page.getByRole("button", { name: "Аккаунты", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Аккаунты" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Создать аккаунт" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Профиль и контекст" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Сменить пароль" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Сбросить сессии" })).toBeVisible();
	await expect(page.getByRole("heading", { name: "Администратор" })).toBeVisible();

	await page.getByRole("button", { name: "Чаты", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Чаты" })).toBeVisible();
	await expect(page.getByText("Выбери разговор для просмотра")).toBeVisible();

	await page.getByRole("button", { name: "Задачи разработки", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Задачи разработки" })).toBeVisible();
	await expect(page.getByText("Задач пока нет.")).toBeVisible();

	await page.getByRole("button", { name: "Контекст ответов", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Контекст ответов" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Сохранить" })).toBeVisible();
	await expect(page.getByText("Публичных вопросов пока не было")).toBeVisible();

	await page.getByRole("button", { name: "Ключ DeepSeek", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Ключ DeepSeek" })).toBeVisible();
	await expect(page.getByText("Ключ модели настроен")).toBeVisible();

	await page.getByRole("button", { name: "Mihomo", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Mihomo" })).toBeVisible();
	await expect(page.getByText("Конфигурация Mihomo пуста")).toBeVisible();

	await page.getByRole("button", { name: "План команды", exact: true }).click();
	await expect(page.getByRole("heading", { name: "План команды" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Создать блок" })).toBeVisible();

	await page.getByRole("button", { name: "Merge request", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Merge request" })).toBeVisible();
	await expect(page.getByText("У задач пока нет merge request")).toBeVisible();

	await page.getByRole("button", { name: "Развёртывания", exact: true }).click();
	await expect(page.getByRole("heading", { name: "Развёртывания" })).toBeVisible();
	await expect(page.getByRole("button", { name: "Развернуть" })).toBeVisible();
	await expect(page.getByText("Операций развёртывания пока нет")).toBeVisible();

	expect(errors).toEqual([]);
});

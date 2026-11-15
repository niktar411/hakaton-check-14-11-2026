import { defineConfig } from "@playwright/test";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const installedBun = join(homedir(), ".bun", "bin", process.platform === "win32" ? "bun.exe" : "bun");
const bun = JSON.stringify(existsSync(installedBun) ? installedBun : "bun");

export default defineConfig({
	testDir: "./tests/browser",
	fullyParallel: false,
	workers: 1,
	use: {
		baseURL: "http://127.0.0.1:4173",
		viewport: { width: 1280, height: 900 },
	},
	webServer: {
		command: `${bun} run build:client && ${bun} x vite preview --config client/vite.config.ts --host 127.0.0.1 --port 4173`,
		url: "http://127.0.0.1:4173",
		reuseExistingServer: false,
		timeout: 120000,
	},
});

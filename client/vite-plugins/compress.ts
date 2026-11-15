import { gzipSync } from "node:zlib";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Plugin } from "vite";

/**
 * Pre-compresses the built bundle and writes a size report, with zero
 * dependencies: node:zlib generates .gz siblings for nginx `gzip_static`.
 *
 * Brotli is intentionally not emitted: the nginx image
 * (nginx:1.29.3-alpine) ships without ngx_brotli, so .br files would be
 * dead weight. If a brotli-enabled image is introduced, emit .br next to
 * .gz here and enable `brotli_static on` in client/nginx.conf.
 *
 * The report (dist/bundle-report.json) replaces rollup-plugin-visualizer:
 * same feedback, no dev dependency. gz sizes in the report are the bytes
 * the browser will actually download.
 */
const COMPRESSIBLE = new Set([
	".html",
	".css",
	".js",
	".mjs",
	".svg",
	".json",
	".txt",
	".xml",
	".map",
]);

interface Entry {
	file: string;
	raw: number;
	gz: number;
}

function walk(dir: string, entries: Entry[]): void {
	for (const name of readdirSync(dir)) {
		const path = join(dir, name);
		const stat = statSync(path);
		if (stat.isDirectory()) {
			walk(path, entries);
			continue;
		}
		const ext = name.slice(name.lastIndexOf("."));
		if (!COMPRESSIBLE.has(ext)) continue;
		const raw = readFileSync(path);
		writeFileSync(`${path}.gz`, gzipSync(raw, { level: 9 }));
		entries.push({ file: path, raw: raw.byteLength, gz: statSync(`${path}.gz`).size });
	}
}

export function compressStatic(distDir: string): Plugin {
	return {
		name: "hackathon-compress-static",
		apply: "build",
		closeBundle() {
			const entries: Entry[] = [];
			walk(distDir, entries);
			entries.sort((a, b) => b.gz - a.gz);
			writeFileSync(
				join(distDir, "bundle-report.json"),
				JSON.stringify({ entries }, null, "\t"),
			);
			const top = entries.slice(0, 10);
			const totalGz = entries.reduce((sum, e) => sum + e.gz, 0);
			// biome-ignore lint/suspicious/noConsole: отчёт сборки печатается в stdout vite-плагином
			console.log(
				`[compress-static] ${entries.length} files, ${Math.round(totalGz / 1024)} KiB gz total. Largest (gz):`,
			);
			for (const e of top) {
				const saved = e.raw > 0 ? Math.round((1 - e.gz / e.raw) * 100) : 0;
				// biome-ignore lint/suspicious/noConsole: отчёт сборки печатается в stdout vite-плагином
				console.log(
					`  ${e.file.slice(distDir.length + 1)}: ${Math.round(e.gz / 1024)} KiB (-${saved}%)`,
				);
			}
		},
	};
}

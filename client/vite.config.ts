import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
import tailwind from "@tailwindcss/vite";
import { paraglideVitePlugin } from "@inlang/paraglide-js";
import { fileURLToPath } from "node:url";
import { compressStatic } from "./vite-plugins/compress";

const distDir = fileURLToPath(new URL("../dist", import.meta.url));
export default defineConfig({
	root: "client",
	resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
	plugins: [
		paraglideVitePlugin({
			project: fileURLToPath(new URL("./project.inlang", import.meta.url)),
			outdir: fileURLToPath(new URL("./src/paraglide", import.meta.url)),
			strategy: ["baseLocale"],
		}),
		solid(),
		tailwind(),
		compressStatic(distDir),
	],
	build: {
		outDir: "../dist",
		emptyOutDir: true,
		// Modern baseline: shorter strings, fewer polyfills, smaller output.
		target: "baseline-widely-available",
		rollupOptions: {
			output: {
				// Long-lived vendor chunks: app code changes churn only its own
				// hashed files, so returning visitors reuse the cached vendor.
				manualChunks(id: string) {
					if (!id.includes("node_modules")) return undefined;
					if (id.includes("solid-js") || id.includes("@solidjs/router")) {
						return "solid";
					}
					if (id.includes("inlang")) return "paraglide";
					return "vendor";
				},
			},
		},
	},
});

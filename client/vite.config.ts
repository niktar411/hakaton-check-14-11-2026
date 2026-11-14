import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
import tailwind from "@tailwindcss/vite";
import { paraglideVitePlugin } from "@inlang/paraglide-js";
import { fileURLToPath } from "node:url";
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
	],
	build: { outDir: "../dist", emptyOutDir: true },
});

import { createSignal, onMount } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

type Theme = "light" | "dark";
const STORAGE_KEY = "portal-theme";
const WAVE_DURATION_MS = 1500;
let waveId = 0;
let waveTimer: number | undefined;
let pendingTheme: Theme | undefined;

function readTheme(): Theme {
	const stored = localStorage.getItem(STORAGE_KEY);
	if (stored === "light" || stored === "dark") return stored;
	return window.matchMedia("(prefers-color-scheme: dark)").matches
		? "dark"
		: "light";
}

function applyTheme(theme: Theme) {
	document.documentElement.dataset.theme = theme;
	try {
		localStorage.setItem(STORAGE_KEY, theme);
	} catch {
		// Theme still applies for this page if storage is unavailable.
	}
	const button = document.querySelector<HTMLButtonElement>("[data-theme-toggle]");
	if (button) {
		const label = theme === "dark" ? m.themeLight() : m.themeDark();
		button.textContent = label;
		button.setAttribute("aria-label", `${label} — ${m.themeToggle()}`);
	}
}

function clearWave(root: HTMLElement) {
	root.classList.remove("theme-wave");
	root.style.removeProperty("--theme-wave-x");
	root.style.removeProperty("--theme-wave-y");
	waveTimer = undefined;
	pendingTheme = undefined;
}

export function ThemeToggle() {
	const [theme, setTheme] = createSignal<Theme>(readTheme());
	onMount(() => applyTheme(theme()));

	const toggle = (event: MouseEvent) => {
		const next = (pendingTheme ?? theme()) === "dark" ? "light" : "dark";
		const root = document.documentElement;
		const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
		const id = ++waveId;

		if (waveTimer !== undefined) window.clearTimeout(waveTimer);
		root.classList.remove("theme-wave");

		if (reducedMotion) {
			pendingTheme = undefined;
			setTheme(next);
			applyTheme(next);
			root.style.removeProperty("--theme-wave-x");
			root.style.removeProperty("--theme-wave-y");
			return;
		}

		// Reflow restarts the same radial wave on every click, including quick repeats.
		void root.offsetWidth;
		root.style.setProperty("--theme-wave-x", `${event.clientX}px`);
		root.style.setProperty("--theme-wave-y", `${event.clientY}px`);
		root.classList.add("theme-wave");
		void root.offsetWidth;

		pendingTheme = next;
		setTheme(next);
		applyTheme(next);
		waveTimer = window.setTimeout(() => {
			if (id === waveId) clearWave(root);
		}, WAVE_DURATION_MS);
	};

	return (
		<Button
			data-theme-toggle="true"
			aria-label={`${theme() === "dark" ? m.themeLight() : m.themeDark()} — ${m.themeToggle()}`}
			onClick={toggle}
		>
			{theme() === "dark" ? m.themeLight() : m.themeDark()}
		</Button>
	);
}

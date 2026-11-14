import { createSignal, onMount } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

type Theme = "light" | "dark";
type ViewTransition = {
	ready: Promise<void>;
	finished: Promise<void>;
	skipTransition: () => void;
};
type ViewTransitionDocument = Document & {
	startViewTransition?: (callback: () => void) => ViewTransition;
};
const STORAGE_KEY = "portal-theme";
let activeTransition: ViewTransition | undefined;
let transitionId = 0;
let pendingTheme: Theme | undefined;
let fallbackTimer: number | undefined;

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
	const toggleButton = document.querySelector<HTMLButtonElement>("[data-theme-toggle]");
	if (toggleButton) {
		const label = theme === "dark" ? m.themeLight() : m.themeDark();
		toggleButton.textContent = label;
		toggleButton.setAttribute("aria-label", `${label} — ${m.themeToggle()}`);
	}
}

export function ThemeToggle() {
	const [theme, setTheme] = createSignal<Theme>(readTheme());
	onMount(() => applyTheme(theme()));
	const toggle = (event: MouseEvent) => {
		const current = pendingTheme ?? theme();
		const next = current === "dark" ? "light" : "dark";
		const root = document.documentElement;
		const viewDocument = document as ViewTransitionDocument;
		const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

		if (!viewDocument.startViewTransition || reducedMotion) {
			pendingTheme = undefined;
			setTheme(next);
			applyTheme(next);
			return;
		}

		const id = ++transitionId;
		pendingTheme = next;
		activeTransition?.skipTransition();
		if (fallbackTimer !== undefined) window.clearTimeout(fallbackTimer);
		root.classList.remove("theme-wave-fallback");
		root.style.setProperty("--theme-wave-x", `${event.clientX}px`);
		root.style.setProperty("--theme-wave-y", `${event.clientY}px`);
		root.classList.add("theme-wave");
		let fallbackStarted = false;
		const fallbackWave = () => {
			if (id !== transitionId || fallbackStarted) return;
			fallbackStarted = true;
			root.classList.remove("theme-wave");
			root.classList.add("theme-wave-fallback");
			setTheme(next);
			applyTheme(next);
			fallbackTimer = window.setTimeout(() => {
				if (id !== transitionId) return;
				root.classList.remove("theme-wave-fallback");
				root.style.removeProperty("--theme-wave-x");
				root.style.removeProperty("--theme-wave-y");
				fallbackTimer = undefined;
			}, 900);
		};
		const transition = viewDocument.startViewTransition(() => {
			if (id !== transitionId) return;
			setTheme(next);
			applyTheme(next);
		});
		activeTransition = transition;
		void transition.ready.catch(fallbackWave);
		const cleanup = () => {
			if (id !== transitionId) return;
			root.classList.remove("theme-wave");
			if (!fallbackStarted) {
				root.style.removeProperty("--theme-wave-x");
				root.style.removeProperty("--theme-wave-y");
			}
			activeTransition = undefined;
			pendingTheme = undefined;
			setTheme(next);
			applyTheme(next);
		};
		void transition.finished.then(cleanup, () => {
			fallbackWave();
			cleanup();
		});
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

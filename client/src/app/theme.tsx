import { createSignal, onMount } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

type Theme = "light" | "dark";
type ViewTransition = {
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

function readTheme(): Theme {
	const stored = localStorage.getItem(STORAGE_KEY);
	if (stored === "light" || stored === "dark") return stored;
	return window.matchMedia("(prefers-color-scheme: dark)").matches
		? "dark"
		: "light";
}

function applyTheme(theme: Theme) {
	document.documentElement.dataset.theme = theme;
	localStorage.setItem(STORAGE_KEY, theme);
}

export function ThemeToggle() {
	const [theme, setTheme] = createSignal<Theme>(readTheme());
	const [labelTheme, setLabelTheme] = createSignal<Theme>(theme());
	let toggleButton: HTMLButtonElement | undefined;
	const syncButton = (value: Theme) => {
		if (!toggleButton) return;
		const label = value === "dark" ? m.themeLight() : m.themeDark();
		toggleButton.textContent = label;
		toggleButton.setAttribute("aria-label", `${label} — ${m.themeToggle()}`);
	};
	onMount(() => {
		setLabelTheme(theme());
		applyTheme(theme());
		syncButton(theme());
	});
	const toggle = (event: MouseEvent) => {
		const current = pendingTheme ?? theme();
		const next = current === "dark" ? "light" : "dark";
		const root = document.documentElement;
		const viewDocument = document as ViewTransitionDocument;
		const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
		setTheme(next);

		if (!viewDocument.startViewTransition || reducedMotion) {
			pendingTheme = undefined;
			setLabelTheme(next);
			applyTheme(next);
			syncButton(next);
			return;
		}

		const id = ++transitionId;
		pendingTheme = next;
		activeTransition?.skipTransition();
		root.style.setProperty("--theme-wave-x", `${event.clientX}px`);
		root.style.setProperty("--theme-wave-y", `${event.clientY}px`);
		root.classList.add("theme-wave");
		const transition = viewDocument.startViewTransition(() => {
			if (id !== transitionId) return;
			setTheme(next);
			setLabelTheme(next);
			applyTheme(next);
			syncButton(next);
		});
		activeTransition = transition;
		const cleanup = () => {
			if (id !== transitionId) return;
			root.classList.remove("theme-wave");
			root.style.removeProperty("--theme-wave-x");
			root.style.removeProperty("--theme-wave-y");
			activeTransition = undefined;
			pendingTheme = undefined;
			setTheme(next);
			setLabelTheme(next === "dark" ? "light" : "dark");
			setLabelTheme(next);
			applyTheme(next);
			syncButton(next);
		};
		void transition.finished.then(cleanup, cleanup);
	};
	return (
		<Button
			ref={(element) => (toggleButton = element)}
			aria-label={`${labelTheme() === "dark" ? m.themeLight() : m.themeDark()} — ${m.themeToggle()}`}
			onClick={toggle}
		>
			{labelTheme() === "dark" ? m.themeLight() : m.themeDark()}
		</Button>
	);
}

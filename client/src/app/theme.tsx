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
	onMount(() => applyTheme(theme()));
	const toggle = (event: MouseEvent) => {
		const next = theme() === "dark" ? "light" : "dark";
		const root = document.documentElement;
		const viewDocument = document as ViewTransitionDocument;
		const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
		const update = () => {
			setTheme(next);
			applyTheme(next);
		};

		if (!viewDocument.startViewTransition || reducedMotion) {
			update();
			return;
		}

		const id = ++transitionId;
		activeTransition?.skipTransition();
		root.style.setProperty("--theme-wave-x", `${event.clientX}px`);
		root.style.setProperty("--theme-wave-y", `${event.clientY}px`);
		root.classList.add("theme-wave");
		const transition = viewDocument.startViewTransition(update);
		activeTransition = transition;
		const cleanup = () => {
			if (id !== transitionId) return;
			root.classList.remove("theme-wave");
			root.style.removeProperty("--theme-wave-x");
			root.style.removeProperty("--theme-wave-y");
			activeTransition = undefined;
		};
		void transition.finished.then(cleanup, cleanup);
	};
	return (
		<Button
			aria-label={`${theme() === "dark" ? m.themeLight() : m.themeDark()} — ${m.themeToggle()}`}
			onClick={toggle}
		>
			{theme() === "dark" ? m.themeLight() : m.themeDark()}
		</Button>
	);
}

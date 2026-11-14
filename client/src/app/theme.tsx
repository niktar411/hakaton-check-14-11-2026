import { createSignal, onMount } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

type Theme = "light" | "dark";
const STORAGE_KEY = "portal-theme";

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
	const toggle = () => {
		const next = theme() === "dark" ? "light" : "dark";
		setTheme(next);
		applyTheme(next);
	};
	return (
		<Button aria-label={m.themeToggle()} onClick={toggle}>
			{theme() === "dark" ? m.themeLight() : m.themeDark()}
		</Button>
	);
}

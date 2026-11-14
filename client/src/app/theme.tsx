import { createSignal } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

export type Theme = "light" | "dark";

const STORAGE_KEY = "portal-theme";

function storedTheme(): Theme | null {
	try {
		const value = localStorage.getItem(STORAGE_KEY);
		return value === "light" || value === "dark" ? value : null;
	} catch {
		return null;
	}
}

function preferredTheme(): Theme {
	if (typeof window === "undefined") return "light";
	const stored = storedTheme();
	if (stored) return stored;
	return window.matchMedia("(prefers-color-scheme: dark)").matches
		? "dark"
		: "light";
}

function applyTheme(theme: Theme, persist: boolean) {
	document.documentElement.dataset.theme = theme;
	if (!persist) return;
	try {
		localStorage.setItem(STORAGE_KEY, theme);
	} catch {
		// Storage can be disabled; the theme still works for this page.
	}
}

/** Applies the saved or system theme before Solid renders the first pixel. */
export function initTheme() {
	if (typeof window === "undefined") return;
	applyTheme(preferredTheme(), false);
}

export function ThemeToggle() {
	const [theme, setTheme] = createSignal<Theme>(preferredTheme());
	const toggle = () => {
		const next = theme() === "dark" ? "light" : "dark";
		setTheme(next);
		applyTheme(next, true);
	};
	return (
		<Button
			aria-label={m.themeToggle()}
			aria-pressed={theme() === "dark"}
			onClick={toggle}
		>
			{theme() === "dark" ? m.themeLight() : m.themeDark()}
		</Button>
	);
}

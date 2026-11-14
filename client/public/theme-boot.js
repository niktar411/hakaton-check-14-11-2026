(() => {
	const key = "portal-theme";
	let theme = "light";
	try {
		const stored = localStorage.getItem(key);
		if (stored === "light" || stored === "dark") {
			theme = stored;
		} else if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
			theme = "dark";
		}
	} catch {
		theme = "light";
	}
	document.documentElement.dataset.theme = theme;
})();

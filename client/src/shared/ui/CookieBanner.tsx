import { A } from "@solidjs/router";
import { createSignal, onMount, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";
import { readConsent, writeConsent } from "@/shared/lib/consent";
import s from "./CookieBanner.module.css";

export function CookieBanner() {
	const [visible, setVisible] = createSignal(false);

	onMount(() => {
		setVisible(readConsent() === null);
	});

	function choose(analytics: boolean) {
		writeConsent(analytics);
		setVisible(false);
	}

	return (
		<Show when={visible()}>
			<section class={s.root} role="dialog" aria-modal="false" aria-label={m.cookieBannerTitle()}>
				<div class={s.text}>
					<h2 class={s.title}>{m.cookieBannerTitle()}</h2>
					<p class={s.lead}>{m.cookieBannerText()}</p>
				</div>
				<div class={s.actions}>
					<Button variant="secondary" onClick={() => choose(false)}>
						{m.cookieBannerNecessary()}
					</Button>
					<Button variant="primary" onClick={() => choose(true)}>
						{m.cookieBannerAll()}
					</Button>
					<A class={s.more} href="/cookies">
						{m.cookieBannerMore()}
					</A>
				</div>
			</section>
		</Show>
	);
}

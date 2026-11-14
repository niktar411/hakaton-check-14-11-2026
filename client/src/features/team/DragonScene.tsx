import * as m from "@/paraglide/messages";
import { cx } from "@/shared/lib/cx";
import { DRAGON_HEAD_PATH } from "@/shared/ui/DragonMark";
import s from "./DragonScene.module.css";

/**
 * Monochrome dragon emblem for the team hero.
 *
 * The head is the "dragon-head" icon by Delapouite from game-icons.net
 * (CC BY 3.0). It is inlined so it follows the text colour of the
 * black-and-white page, scales without raster assets and stays crisp on every
 * screen. The attribution link under the emblem is required by the licence.
 */
export function DragonScene(props: { class?: string }) {
	return (
		<div class={cx(s.stage, props.class)}>
			<span class={s.halo} aria-hidden="true" />
			<span class={s.ring} aria-hidden="true" />
			<span class={s.ringInner} aria-hidden="true" />
			<svg
				class={s.dragon}
				viewBox="0 0 512 512"
				role="presentation"
				aria-hidden="true"
			>
				<defs>
					<linearGradient id="team-dragon-mono" x1="0" y1="0" x2="0.6" y2="1">
						<stop class={s.stopInk} offset="0" />
						<stop class={s.stopFade} offset="1" />
					</linearGradient>
				</defs>
				<path class={s.head} d={DRAGON_HEAD_PATH} />
			</svg>
			<a
				class={s.credit}
				href="https://game-icons.net/1x1/delapouite/dragon-head.html"
				target="_blank"
				rel="noopener noreferrer"
			>
				{m.teamDragonCredit()}
			</a>
		</div>
	);
}

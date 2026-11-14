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
					<linearGradient id="team-dragon-fire" x1="0" y1="0" x2="1" y2="0">
						<stop class={s.fireCore} offset="0" />
						<stop class={s.fireMid} offset="0.55" />
						<stop class={s.fireEdge} offset="1" />
					</linearGradient>
				</defs>
				<path class={s.head} d={DRAGON_HEAD_PATH} />
				<g>
					<path
						class={s.mouthOpening}
						d="M376 322c20 2 38-1 56-9-7 20-25 30-44 25-8-2-12-7-12-16z"
					/>
					<path
						class={s.flameOuter}
						d="M405 328c22-6 32-22 62-28-9 9-14 17-9 22 7 7 23-8 46-12-9 12-14 19-10 24 4 5 14 2 26-6-8 21-25 33-48 31-26-2-49-12-67-31z"
					/>
					<path
						class={s.flameInner}
						d="M408 329c22-7 36-14 54-17-7 7-7 13 0 16 8 3 21-3 34-8-12 14-29 18-45 15-17-1-31-3-43-6z"
					/>
				</g>
			</svg>
		</div>
	);
}

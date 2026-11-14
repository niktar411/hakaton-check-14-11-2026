import { cx } from "@/shared/lib/cx";
import s from "./DragonScene.module.css";

/**
 * Decorative dragon emblem for the team hero. It is drawn inline so it follows
 * the light and dark tokens, scales without raster assets and can be animated
 * with the rest of the page.
 */
export function DragonScene(props: { class?: string }) {
	return (
		<div class={cx(s.stage, props.class)} aria-hidden="true">
			<span class={s.halo} />
			<span class={s.ring} />
			<span class={s.ringInner} />
			<svg class={s.dragon} viewBox="0 0 480 480" role="presentation">
				<defs>
					<linearGradient id="team-dragon-body" x1="0" y1="0" x2="1" y2="1">
						<stop class={s.stopPrimary} offset="0" />
						<stop class={s.stopTeal} offset="1" />
					</linearGradient>
					<linearGradient id="team-dragon-wing" x1="0" y1="0" x2="1" y2="1">
						<stop class={s.stopPrimarySoft} offset="0" />
						<stop class={s.stopSurface} offset="1" />
					</linearGradient>
				</defs>

				<path
					class={s.wingBack}
					d="M 274 250 C 228 204 180 154 120 108 C 134 166 146 216 170 254 C 198 286 236 298 270 292 Z"
				/>
				<path class={s.wingRib} d="M 264 278 C 236 250 206 220 170 190" />
				<path class={s.wingRib} d="M 252 286 C 226 264 198 242 166 226" />

				<path
					class={s.tail}
					d="M 292 228 C 270 268 252 310 224 344 C 194 380 150 402 108 412 C 92 416 92 432 108 434 C 164 440 216 412 250 376 C 278 346 300 308 316 270 Z"
				/>
				<path
					class={s.tailSpade}
					d="M 108 412 C 84 394 60 384 36 388 C 50 402 58 418 58 438 C 74 424 90 422 108 434 Z"
				/>

				<path
					class={s.leg}
					d="M 292 296 C 308 314 314 334 308 352 L 324 358 C 332 334 324 306 306 286 Z"
				/>
				<path class={s.claw} d="M 308 352 L 298 368 M 316 354 L 314 372 M 323 355 L 328 370" />

				<path class={s.spike} d="M 292 232 L 300 204 L 308 238 Z" />
				<path class={s.spike} d="M 272 262 L 284 232 L 290 268 Z" />
				<path class={s.spike} d="M 252 292 L 268 262 L 272 300 Z" />
				<path class={s.spike} d="M 230 324 L 248 294 L 250 332 Z" />

				<path
					class={s.head}
					d="M 258 226 C 276 196 306 180 342 180 L 392 188 L 448 208 C 458 212 458 226 446 230 L 396 240 L 360 256 L 314 250 C 292 246 272 238 258 226 Z"
				/>
				<path
					class={s.mouth}
					d="M 330 250 C 366 246 404 240 440 232 L 436 248 C 400 254 362 260 334 264 Z"
				/>
				<path
					class={s.jaw}
					d="M 326 252 L 440 238 C 452 240 454 250 442 256 L 352 270 C 334 272 320 264 326 252 Z"
				/>
				<path
					class={s.teeth}
					d="M 344 250 L 350 240 L 356 249 L 362 238 L 368 248 L 374 236 L 380 247 L 386 235 L 392 246"
				/>
				<path
					class={s.teeth}
					d="M 348 258 L 352 266 L 358 258 L 364 266 L 370 258 L 376 266 L 382 258"
				/>

				<path class={s.horn} d="M 330 184 C 306 158 280 140 242 128 C 270 148 290 166 300 192 Z" />
				<path class={s.horn} d="M 356 186 C 336 160 312 144 280 132 C 306 152 320 172 328 196 Z" />
				<path class={s.fin} d="M 318 200 C 298 190 282 194 270 208 C 286 210 300 214 312 222 Z" />

				<path
					class={s.wingFront}
					d="M 306 268 C 280 214 242 162 188 112 C 198 170 204 222 220 268 C 232 294 258 308 292 308 Z"
				/>
				<path class={s.wingRib} d="M 300 272 C 278 236 254 202 220 166" />
				<path class={s.wingRib} d="M 292 280 C 270 254 248 228 220 202" />

				<path class={s.brow} d="M 366 194 C 376 190 388 192 394 198" />
				<circle class={s.eyeGlow} cx="386" cy="208" r="13" />
				<circle class={s.eye} cx="386" cy="208" r="6" />
				<circle class={s.nostril} cx="430" cy="216" r="3" />

				<path
					class={s.flameOuter}
					d="M 448 218 C 464 212 474 200 476 184 C 480 206 474 226 456 236 C 462 222 460 206 452 196 C 452 206 451 212 448 218 Z"
				/>
				<path
					class={s.flameInner}
					d="M 450 219 C 460 215 467 207 470 197 C 472 210 466 222 455 229 C 459 219 456 210 450 219 Z"
				/>
				<circle class={s.ember} cx="446" cy="228" r="3.5" />
				<circle class={cx(s.ember, s.ember2)} cx="458" cy="208" r="2.5" />
				<circle class={cx(s.ember, s.ember3)} cx="438" cy="198" r="2" />
				<path class={s.sparkLine} d="M 438 214 C 456 204 464 188 468 168" />
			</svg>
		</div>
	);
}

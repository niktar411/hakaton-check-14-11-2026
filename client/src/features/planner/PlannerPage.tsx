import { useNavigate } from "@solidjs/router";
import {
	createEffect,
	createMemo,
	createResource,
	createSignal,
	For,
	onCleanup,
	onMount,
	Show,
} from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { safeColor } from "@/shared/lib/color";
import { formatDate } from "@/shared/lib/format";
import { Dialog } from "@/shared/ui/Dialog";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./PlannerPage.module.css";

type Progress = components["schemas"]["Progress"];
type PlannerBlock = components["schemas"]["PlannerBlock"];
type PublicBlock = components["schemas"]["PublicPlannerBlock"];
type AnyBlock = PlannerBlock | PublicBlock;

type PlannerView =
	| { team: true; username: string; csrf: string; blocks: PlannerBlock[] }
	| { team: false; blocks: PublicBlock[] };

type LayoutNode = {
	block: AnyBlock;
	children: LayoutNode[];
	depth: number;
	x: number;
	y: number;
};

type Edge = {
	id: string;
	path: string;
	done: boolean;
	endX: number;
	endY: number;
};

type Spark = { x: number; y: number };

const SPARKS: Spark[] = Array.from({ length: 12 }, (_, index) => {
	const angle = (Math.PI * 2 * index) / 12;
	const radius = index % 3 === 0 ? 34 : index % 2 === 0 ? 26 : 20;
	return { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius };
});

type Layout = {
	nodes: LayoutNode[];
	edges: Edge[];
	width: number;
	height: number;
	rootX: number;
	rootY: number;
	cardWidth: number;
	cardHeight: number;
};

const CARD = {
	public: { width: 220, height: 104 },
	team: { width: 286, height: 172 },
};
const PADDING = 90;

function buildLayout(blocks: AnyBlock[], team: boolean): Layout {
	const card = team ? CARD.team : CARD.public;
	const gapX = team ? 96 : 80;
	const gapY = team ? 28 : 22;
	const byId = new Map<number, LayoutNode>();
	for (const block of blocks) {
		byId.set(block.id, { block, children: [], depth: 0, x: 0, y: 0 });
	}
	const roots: LayoutNode[] = [];
	for (const node of byId.values()) {
		const parent =
			node.block.parentId === null || node.block.parentId === undefined
				? undefined
				: byId.get(node.block.parentId);
		if (parent && parent !== node) {
			parent.children.push(node);
		} else {
			roots.push(node);
		}
	}
	const byPosition = (a: LayoutNode, b: LayoutNode) =>
		a.block.position - b.block.position || a.block.id - b.block.id;
	for (const node of byId.values()) {
		node.children.sort(byPosition);
	}
	roots.sort(byPosition);

	const leaves = new Map<LayoutNode, number>();
	const countLeaves = (node: LayoutNode): number => {
		const known = leaves.get(node);
		if (known !== undefined) return known;
		const count = node.children.length
			? node.children.reduce((sum, child) => sum + countLeaves(child), 0)
			: 1;
		leaves.set(node, count);
		return count;
	};

	const root = roots.length === 1 ? roots[0] : undefined;
	const nodes: LayoutNode[] = [];

	// A side is a vertical stack of subtrees. Every child sits in a column one
	// level farther from the root, so all connectors can be drawn as horizontal
	// and vertical segments with right angles only.
	const placeSide = (sideRoots: LayoutNode[], direction: 1 | -1) => {
		let cursorY = 0;
		const placed: LayoutNode[] = [];
		const place = (node: LayoutNode, depth: number) => {
			node.depth = depth;
			node.x = direction * depth * (card.width + gapX);
			if (node.children.length === 0) {
				node.y = cursorY + card.height / 2;
				cursorY += card.height + gapY;
			} else {
				for (const child of node.children) {
					place(child, depth + 1);
				}
				const first = node.children[0];
				const last = node.children[node.children.length - 1];
				node.y = (first.y + last.y) / 2;
			}
			placed.push(node);
		};
		for (const child of sideRoots) {
			place(child, 1);
			cursorY += gapY;
		}
		const center = cursorY / 2;
		for (const node of placed) {
			node.y -= center;
		}
		nodes.push(...placed);
	};

	if (root) {
		root.depth = 0;
		root.x = 0;
		root.y = 0;
		const right: LayoutNode[] = [];
		const left: LayoutNode[] = [];
		let rightLeaves = 0;
		let leftLeaves = 0;
		for (const child of root.children) {
			const weight = countLeaves(child);
			if (rightLeaves <= leftLeaves) {
				right.push(child);
				rightLeaves += weight;
			} else {
				left.push(child);
				leftLeaves += weight;
			}
		}
		placeSide(right, 1);
		placeSide(left, -1);
		nodes.push(root);
	} else {
		// A malformed forest still gets one readable stack.
		placeSide(roots, 1);
	}

	const minX = Math.min(...nodes.map((node) => node.x - card.width / 2));
	const maxX = Math.max(...nodes.map((node) => node.x + card.width / 2));
	const minY = Math.min(...nodes.map((node) => node.y - card.height / 2));
	const maxY = Math.max(...nodes.map((node) => node.y + card.height / 2));
	const shiftX = PADDING - minX;
	const shiftY = PADDING - minY;
	for (const node of nodes) {
		node.x += shiftX;
		node.y += shiftY;
	}
	const width = maxX - minX + PADDING * 2;
	const height = maxY - minY + PADDING * 2;

	const edges: Edge[] = [];
	for (const node of nodes) {
		for (const child of node.children) {
			const id = `${node.block.id}-${child.block.id}`;
			const done = child.block.progress.state === "done";
			if (child.x > node.x + 0.5) {
				const startX = node.x + card.width / 2;
				const endX = child.x - card.width / 2;
				const middleX = (startX + endX) / 2;
				edges.push({
					id,
					path: `M ${startX} ${node.y} H ${middleX} V ${child.y} H ${endX}`,
					done,
					endX,
					endY: child.y,
				});
			} else if (child.x < node.x - 0.5) {
				const startX = node.x - card.width / 2;
				const endX = child.x + card.width / 2;
				const middleX = (startX + endX) / 2;
				edges.push({
					id,
					path: `M ${startX} ${node.y} H ${middleX} V ${child.y} H ${endX}`,
					done,
					endX,
					endY: child.y,
				});
			} else {
				const direction = child.y >= node.y ? 1 : -1;
				const startY = node.y + (direction * card.height) / 2;
				const endY = child.y - (direction * card.height) / 2;
				const middleY = (startY + endY) / 2;
				edges.push({
					id,
					path: `M ${node.x} ${startY} V ${middleY} H ${child.x} V ${endY}`,
					done,
					endX: child.x,
					endY,
				});
			}
		}
	}

	const anchor = root ?? roots[0];
	return {
		nodes,
		edges,
		width,
		height,
		rootX: anchor ? anchor.x : width / 2,
		rootY: anchor ? anchor.y : height / 2,
		cardWidth: card.width,
		cardHeight: card.height,
	};
}

export function plannerStatus(status: Progress["state"]) {
	switch (status) {
		case "planned":
			return m.plannerStatusPlanned();
		case "verifying":
			return m.plannerStatusVerifying();
		case "failed":
			return m.plannerStatusFailed();
		case "review":
			return m.plannerStatusReview();
		case "done":
			return m.plannerStatusDone();
		case "unknown":
			return m.unknown();
	}
}

function PlannerNode(props: {
	node: LayoutNode;
	team: boolean;
	onSelect: (block: AnyBlock) => void;
	suppressClick: () => boolean;
}) {
	const block = () => props.node.block;
	const card = props.team ? CARD.team : CARD.public;
	const visible = () => props.team || block().progress.state === "done";
	const inProgress = () =>
		block().progress.state === "verifying" || block().progress.state === "review";
	return (
		<article
			class={s.nodeCard}
			data-root={props.node.depth === 0}
			data-empty={!visible()}
			data-clickable={visible()}
			data-active={inProgress()}
			role={visible() ? "button" : undefined}
			tabIndex={visible() ? 0 : undefined}
			title={visible() ? m.plannerOpenDetails() : undefined}
			onClick={(event) => {
				if (!visible() || props.suppressClick()) return;
				if ((event.target as HTMLElement).closest("a, button")) return;
				props.onSelect(block());
			}}
			onKeyDown={(event) => {
				if (!visible()) return;
				if (event.key !== "Enter" && event.key !== " ") return;
				event.preventDefault();
				props.onSelect(block());
			}}
			style={{
				left: `${props.node.x - card.width / 2}px`,
				top: `${props.node.y - card.height / 2}px`,
				width: `${card.width}px`,
				height: `${card.height}px`,
				"--owner-color":
					props.team && "owner" in block()
						? safeColor((block() as PlannerBlock).owner.color)
						: "var(--text-muted)",
			}}
		>
			<Show when={inProgress()}>
				<svg class={s.activeBorder} aria-hidden="true">
					<rect class={s.activeBorderRect} />
				</svg>
			</Show>
			<Show when={visible()}>
				<div class={s.blockHeader}>
					<h2>
						<span class={s.blockNumber}>
							{m.plannerBlockNumber({ number: String(block().id) })}
						</span>
						{block().title}
					</h2>
					<span class={s.status} data-status={block().progress.state}>
						{plannerStatus(block().progress.state)}
					</span>
				</div>
			<Show
				when={props.team && "owner" in block()}
				fallback={
					<dl class={s.nodeMeta}>
						<div>
							<dt>{m.plannerUpdated()}</dt>
							<dd>
								<time datetime={block().updatedAt}>
									{formatDate(block().updatedAt)}
								</time>
							</dd>
						</div>
					</dl>
				}
			>
				{(() => {
					const item = block() as PlannerBlock;
					return (
						<>
							<p class={s.owner}>
								{item.owner.displayName} · {item.owner.username}
							</p>
							<Show when={item.description}>
								<p class={s.nodeDescription}>{item.description}</p>
							</Show>
							<dl class={s.nodeMeta}>
								<div>
									<dt>{m.plannerUpdated()}</dt>
									<dd>
										<time datetime={item.updatedAt}>
											{formatDate(item.updatedAt)}
										</time>
									</dd>
								</div>
								<Show when={item.branch}>
									<div>
										<dt>{m.plannerBranch()}</dt>
										<dd>{item.branch}</dd>
									</div>
								</Show>
								<Show when={item.progress.pipelineStatus}>
									<div>
										<dt>{m.pipelineStatus()}</dt>
										<dd>{item.progress.pipelineStatus}</dd>
									</div>
								</Show>
								<Show when={item.progress.approvals}>
									{(approvals) => (
										<div>
											<dt>{m.plannerApprovalState()}</dt>
											<dd>
												{approvals().approved
													? m.approved()
													: m.pendingApproval()}
											</dd>
										</div>
									)}
								</Show>
							</dl>
							<Show when={item.mrUrl}>
								<a
									href={item.mrUrl}
									target="_blank"
									rel="noopener noreferrer"
								>
									{m.mergeRequestStatus()}
									{item.mrIid ? ` !${item.mrIid}` : ""}
								</a>
							</Show>
						</>
					);
				})()}
			</Show>
			</Show>
		</article>
	);
}

export function PlannerBoard(props: {
	blocks: AnyBlock[];
	team: boolean;
	onSelect: (block: AnyBlock) => void;
	contained?: boolean;
	wheelZoom?: boolean;
}) {
	const [board, setBoard] = createSignal<HTMLDivElement>();
	const [zoom, setZoom] = createSignal(1);
	const [pan, setPan] = createSignal({ x: 0, y: 0 });
	const [dragging, setDragging] = createSignal(false);
	const [dragged, setDragged] = createSignal(false);
	const [celebrating, setCelebrating] = createSignal<Set<string>>(new Set());
	const layout = createMemo(() => buildLayout(props.blocks, props.team));
	let knownDone: Set<string> | undefined;
	let celebrationTimer: number | undefined;
	let drag:
		| {
				pointerId: number;
				x: number;
				y: number;
				panX: number;
				panY: number;
				captured: boolean;
		  }
		| undefined;

	createEffect(() => {
		const done = new Set<string>(
			layout()
				.edges.filter((edge) => edge.done)
				.map((edge) => edge.id),
		);
		if (knownDone === undefined) {
			// Opening the board should not replay the explosion for every edge
			// that was already completed before this visit.
			knownDone = done;
			return;
		}
		const fresh = [...done].filter((id) => !knownDone?.has(id));
		knownDone = done;
		if (!fresh.length) return;
		setCelebrating(new Set<string>(fresh));
		if (celebrationTimer !== undefined) {
			window.clearTimeout(celebrationTimer);
		}
		celebrationTimer = window.setTimeout(() => {
			setCelebrating(new Set<string>());
			celebrationTimer = undefined;
		}, 2400);
	});

	onCleanup(() => {
		if (celebrationTimer !== undefined) {
			window.clearTimeout(celebrationTimer);
		}
	});

	const fit = (showAll = false) => {
		const element = board();
		if (!element) return;
		const rect = element.getBoundingClientRect();
		if (rect.width < 32 || rect.height < 32) return;
		const current = layout();
		const minX = Math.min(
			...current.nodes.map((node) => node.x - current.cardWidth / 2),
		);
		const maxX = Math.max(
			...current.nodes.map((node) => node.x + current.cardWidth / 2),
		);
		const minY = Math.min(
			...current.nodes.map((node) => node.y - current.cardHeight / 2),
		);
		const maxY = Math.max(
			...current.nodes.map((node) => node.y + current.cardHeight / 2),
		);
		const width = Math.max(1, maxX - minX);
		const height = Math.max(1, maxY - minY);
		const allZoom = Math.max(
			0.12,
			Math.min((rect.width - 96) / width, (rect.height - 96) / height, 1.4),
		);
		// Open at a readable scale around the centre. "Show all" zooms out far
		// enough to see every branch; the wheel and +/- go in both directions.
		const next = showAll ? allZoom : Math.max(allZoom, 0.8);
		const centerX = showAll ? (minX + maxX) / 2 : current.rootX;
		const centerY = showAll ? (minY + maxY) / 2 : current.rootY;
		setZoom(next);
		setPan({
			x: rect.width / 2 - centerX * next,
			y: rect.height / 2 - centerY * next,
		});
	};

	const zoomBy = (factor: number, clientX?: number, clientY?: number) => {
		const element = board();
		if (!element) return;
		const rect = element.getBoundingClientRect();
		const pointX = (clientX ?? rect.left + rect.width / 2) - rect.left;
		const pointY = (clientY ?? rect.top + rect.height / 2) - rect.top;
		const previous = zoom();
		const next = Math.min(2.8, Math.max(0.12, previous * factor));
		const current = pan();
		setPan({
			x: pointX - (pointX - current.x) * (next / previous),
			y: pointY - (pointY - current.y) * (next / previous),
		});
		setZoom(next);
	};

	onMount(() => {
		const element = board();
		if (!element) return;
		const wheel = (event: WheelEvent) => {
			event.preventDefault();
			zoomBy(event.deltaY < 0 ? 1.12 : 0.89, event.clientX, event.clientY);
		};
		if (props.wheelZoom !== false) {
			element.addEventListener("wheel", wheel, { passive: false });
		}
		const resetView = () => fit();
		const observer = new ResizeObserver(resetView);
		observer.observe(element);
		resetView();
		onCleanup(() => {
			element.removeEventListener("wheel", wheel);
			observer.disconnect();
		});
	});

	const pointerDown = (event: PointerEvent) => {
		if ((event.target as HTMLElement).closest("a, button")) return;
		setDragged(false);
		drag = {
			pointerId: event.pointerId,
			x: event.clientX,
			y: event.clientY,
			panX: pan().x,
			panY: pan().y,
			captured: false,
		};
		setDragging(true);
	};
	const pointerMove = (event: PointerEvent) => {
		if (!drag || drag.pointerId !== event.pointerId) return;
		if (
			Math.abs(event.clientX - drag.x) > 4 ||
			Math.abs(event.clientY - drag.y) > 4
		) {
			setDragged(true);
			// Capture only after a real drag. Capturing on pointerdown made the
			// browser retarget the click to the board, so card clicks never fired.
			if (!drag.captured) {
				(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
				drag.captured = true;
			}
		}
		setPan({
			x: drag.panX + event.clientX - drag.x,
			y: drag.panY + event.clientY - drag.y,
		});
	};
	const pointerUp = (event: PointerEvent) => {
		if (drag?.captured) {
			(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
		}
		drag = undefined;
		setDragging(false);
	};
	const pointerLeave = () => {
		drag = undefined;
		setDragging(false);
	};

	return (
		<section
			class={s.boardSection}
			classList={{ [s.boardContained]: props.contained === true }}
			aria-label={m.plannerBoard()}
		>
			<div
				class={s.board}
				ref={setBoard}
				data-dragging={dragging()}
				onPointerDown={pointerDown}
				onPointerMove={pointerMove}
				onPointerUp={pointerUp}
				onPointerCancel={pointerUp}
				onPointerLeave={pointerLeave}
			>
				<fieldset
					class={s.zoomControls}
					aria-label={m.plannerZoomControls()}
				>
					<button
						type="button"
						class={s.zoomButton}
						aria-label={m.plannerZoomOut()}
						title={m.plannerZoomOut()}
						onClick={() => zoomBy(0.86)}
					>
						−
					</button>
					<button
						type="button"
						class={s.zoomButton}
						aria-label={m.plannerZoomIn()}
						title={m.plannerZoomIn()}
						onClick={() => zoomBy(1.16)}
					>
						+
					</button>
					<button
						type="button"
						class={s.zoomButton}
						aria-label={m.plannerFit()}
						title={m.plannerFit()}
						onClick={() => fit(true)}
					>
						⤢
					</button>
				</fieldset>
				<div
					class={s.world}
					style={{
						width: `${layout().width}px`,
						height: `${layout().height}px`,
						transform: `translate(${pan().x}px, ${pan().y}px) scale(${zoom()})`,
					}}
				>
					<svg
						class={s.edges}
						width={layout().width}
						height={layout().height}
						viewBox={`0 0 ${layout().width} ${layout().height}`}
						aria-hidden="true"
					>
						<defs>
							<marker
								id="planner-arrow"
								viewBox="0 0 10 10"
								refX="9"
								refY="5"
								markerWidth="6"
								markerHeight="6"
								orient="auto-start-reverse"
							>
								<path d="M 0 0 L 10 5 L 0 10 z" class={s.arrowHead} />
							</marker>
							<marker
								id="planner-arrow-done"
								viewBox="0 0 10 10"
								refX="9"
								refY="5"
								markerWidth="6"
								markerHeight="6"
								orient="auto-start-reverse"
							>
								<path d="M 0 0 L 10 5 L 0 10 z" class={s.arrowHeadDone} />
							</marker>
						</defs>
						<For each={layout().edges}>
							{(edge) => (
								<g
									class={s.edgeGroup}
									data-done={edge.done}
									data-celebrating={celebrating().has(edge.id)}
								>
									<path
										d={edge.path}
										class={s.edge}
										marker-end={
											edge.done ? undefined : "url(#planner-arrow)"
										}
									/>
									<Show when={edge.done && !celebrating().has(edge.id)}>
										<path
											d={edge.path}
											class={s.edgeDone}
											marker-end="url(#planner-arrow-done)"
										/>
									</Show>
									<Show when={celebrating().has(edge.id)}>
										<path
											d={edge.path}
											pathLength={1}
											class={s.edgeDraw}
											marker-end="url(#planner-arrow-done)"
										/>
										<path
											d={edge.path}
											pathLength={1}
											class={s.edgeBulge}
										/>
										<circle r="5" class={s.edgeBall}>
											<animateMotion
												dur="1.2s"
												fill="freeze"
												path={edge.path}
												calcMode="spline"
												keyPoints="0;1"
												keyTimes="0;1"
												keySplines="0.35 0 0.25 1"
											/>
										</circle>
										<g class={s.sparkBurst}>
											<For each={SPARKS}>
												{(spark) => (
													<circle
														cx={edge.endX}
														cy={edge.endY}
														r="2.2"
														class={s.spark}
														style={{
															"--spark-x": `${spark.x}px`,
															"--spark-y": `${spark.y}px`,
														}}
													/>
												)}
											</For>
											<circle
												cx={edge.endX}
												cy={edge.endY}
												r="10"
												class={s.burstRing}
											/>
										</g>
									</Show>
								</g>
							)}
						</For>
					</svg>
					<For each={layout().nodes}>
						{(node) => (
							<PlannerNode
								node={node}
								team={props.team}
								onSelect={props.onSelect}
								suppressClick={() => dragged()}
							/>
						)}
					</For>
				</div>
			</div>
		</section>
	);
}

export default function PlannerPage() {
	const toast = useToast();
	const navigate = useNavigate();
	const [selected, setSelected] = createSignal<AnyBlock>();
	const [starting, setStarting] = createSignal(false);
	const [planner, { refetch }] = createResource(
		async (): Promise<PlannerView> => {
			try {
				const me = unwrap(await api.GET("/api/me"));
				try {
					return {
						team: true,
						username: me.username,
						csrf: me.csrf,
						blocks: unwrap(await api.GET("/api/planner")) ?? [],
					};
				} catch {
					// Fall through to the public projection if the session expired.
				}
			} catch {
				// Anonymous visitor.
			}
			return {
				team: false,
				blocks: unwrap(await api.GET("/api/public/planner")) ?? [],
			};
		},
	);
	const team = () => planner()?.team === true;
	const currentUsername = () => {
		const view = planner();
		return view?.team ? view.username : "";
	};
	createEffect(() => {
		// Anonymous visitors get the plan inside the single public page.
		if (!planner.loading && !team()) {
			navigate("/", { replace: true });
		}
	});
	async function startWork(block: AnyBlock) {
		if (starting()) return;
		setStarting(true);
		try {
			const view = planner();
			const csrf = view?.team ? view.csrf : undefined;
			const updated = unwrap(
				await api.POST("/api/planner/blocks/{id}/start", {
					headers: csrfHeaders(csrf),
					params: { path: { id: block.id } },
				}),
			);
			setSelected(updated);
			await refetch();
			toast.show(m.plannerWorkStarted());
		} catch {
			toast.show(m.loadError());
		} finally {
			setStarting(false);
		}
	}
	onMount(() => {
		const timer = window.setInterval(() => void refetch(), 15000);
		const onFocus = () => void refetch();
		window.addEventListener("focus", onFocus);
		onCleanup(() => {
			window.clearInterval(timer);
			window.removeEventListener("focus", onFocus);
		});
	});
	return (
		<main class={s.page}>
			<Show
				when={!planner.loading || !!planner.latest}
				fallback={<LoadingState />}
			>
				<Show
					when={!planner.error || !!planner.latest}
					fallback={<ErrorState onRetry={() => void refetch()} />}
				>
					<Show when={planner.error}>
						<p class={s.stale} role="status">
							{m.plannerStale()}
						</p>
					</Show>
					<Show
						when={(planner()?.blocks.length ?? 0) > 0}
						fallback={<EmptyState message={m.plannerEmpty()} />}
					>
						<PlannerBoard
							blocks={(planner()?.blocks as AnyBlock[]) ?? []}
							team={team()}
							onSelect={setSelected}
						/>
					</Show>
				</Show>
			</Show>
			<Dialog
				open={!!selected()}
				title={selected()?.title || m.plannerDetails()}
				onClose={() => setSelected(undefined)}
			>
				<Show when={selected()}>
					{(block) => {
						const teamBlock = () =>
							team() && "owner" in block() ? (block() as PlannerBlock) : undefined;
						return (
							<div class="grid min-w-0 gap-4">
								<div class="flex flex-wrap items-center gap-3">
									<span class={s.blockNumber}>
										{m.plannerBlockNumber({ number: String(block().id) })}
									</span>
									<span class={s.status} data-status={block().progress.state}>
										{plannerStatus(block().progress.state)}
									</span>
									<Show when={teamBlock()}>
										{(item) => (
											<span class={s.owner}>
												{item().owner.displayName} · {item().owner.username}
											</span>
										)}
									</Show>
								</div>
								<Show when={teamBlock()}>
									{(item) => (
										<Show when={item().description}>
											<p class={s.description}>{item().description}</p>
										</Show>
									)}
								</Show>
								<Show when={!teamBlock()}>
									<p class="text-text-muted">{m.plannerPublicHint()}</p>
								</Show>
								<dl class={s.dates}>
									<div>
										<dt>{m.plannerUpdated()}</dt>
										<dd>
											<time datetime={block().updatedAt}>
												{formatDate(block().updatedAt)}
											</time>
										</dd>
									</div>
									<Show when={teamBlock()}>
										{(item) => (
											<>
												<Show when={item().branch}>
													<div>
														<dt>{m.plannerBranch()}</dt>
														<dd>{item().branch}</dd>
													</div>
												</Show>
												<Show when={item().progress.pipelineStatus}>
													<div>
														<dt>{m.pipelineStatus()}</dt>
														<dd>{item().progress.pipelineStatus}</dd>
													</div>
												</Show>
												<Show when={item().progress.approvals}>
													{(approvals) => (
														<div>
															<dt>{m.plannerApprovalState()}</dt>
															<dd>
																{approvals().approved
																	? m.approved()
																	: m.pendingApproval()}
															</dd>
														</div>
													)}
												</Show>
											</>
										)}
									</Show>
								</dl>
								<Show
									when={
										teamBlock()?.owner.username === currentUsername() &&
										block().progress.state === "planned"
									}
								>
									<Button
										variant="primary"
										disabled={starting()}
										onClick={() => void startWork(block())}
									>
										{starting() ? m.loading() : m.plannerStartWork()}
									</Button>
								</Show>
								<Show when={teamBlock()?.mrUrl}>
									<a
										href={teamBlock()?.mrUrl}
										target="_blank"
										rel="noopener noreferrer"
									>
										{m.openMergeRequest()}
									</a>
								</Show>
							</div>
						);
					}}
				</Show>
			</Dialog>
		</main>
	);
}

import { A } from "@solidjs/router";
import {
	createMemo,
	createResource,
	createSignal,
	For,
	onCleanup,
	onMount,
	Show,
} from "solid-js";
import * as m from "@/paraglide/messages";
import { useSiteConfig } from "@/app/site-config";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { safeColor } from "@/shared/lib/color";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import s from "./PlannerPage.module.css";

type Progress = components["schemas"]["Progress"];
type PlannerBlock = components["schemas"]["PlannerBlock"];
type PublicBlock = components["schemas"]["PublicPlannerBlock"];
type AnyBlock = PlannerBlock | PublicBlock;

type PlannerView =
	| { team: true; blocks: PlannerBlock[] }
	| { team: false; blocks: PublicBlock[] };

type LayoutNode = {
	block: AnyBlock;
	children: LayoutNode[];
	depth: number;
	x: number;
	y: number;
};

type Edge = { id: string; path: string };

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
			if (child.x > node.x + 0.5) {
				const startX = node.x + card.width / 2;
				const endX = child.x - card.width / 2;
				const middleX = (startX + endX) / 2;
				edges.push({
					id,
					path: `M ${startX} ${node.y} H ${middleX} V ${child.y} H ${endX}`,
				});
			} else if (child.x < node.x - 0.5) {
				const startX = node.x - card.width / 2;
				const endX = child.x + card.width / 2;
				const middleX = (startX + endX) / 2;
				edges.push({
					id,
					path: `M ${startX} ${node.y} H ${middleX} V ${child.y} H ${endX}`,
				});
			} else {
				const direction = child.y >= node.y ? 1 : -1;
				const startY = node.y + (direction * card.height) / 2;
				const endY = child.y - (direction * card.height) / 2;
				const middleY = (startY + endY) / 2;
				edges.push({
					id,
					path: `M ${node.x} ${startY} V ${middleY} H ${child.x} V ${endY}`,
				});
			}
		}
	}

	return {
		nodes,
		edges,
		width,
		height,
		rootX: root ? root.x : width / 2,
		rootY: root ? root.y : height / 2,
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

function PlannerNode(props: { node: LayoutNode; team: boolean }) {
	const block = () => props.node.block;
	const card = props.team ? CARD.team : CARD.public;
	return (
		<article
			class={s.nodeCard}
			data-root={props.node.depth === 0}
			style={{
				left: `${props.node.x - card.width / 2}px`,
				top: `${props.node.y - card.height / 2}px`,
				width: `${card.width}px`,
				height: `${card.height}px`,
				"--owner-color":
					props.team && "owner" in block()
						? safeColor((block() as PlannerBlock).owner.color)
						: "var(--border)",
			}}
		>
			<div class={s.blockHeader}>
				<h2>{block().title}</h2>
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
		</article>
	);
}

function PlannerBoard(props: { blocks: AnyBlock[]; team: boolean }) {
	const [board, setBoard] = createSignal<HTMLDivElement>();
	const [zoom, setZoom] = createSignal(1);
	const [pan, setPan] = createSignal({ x: 0, y: 0 });
	const [dragging, setDragging] = createSignal(false);
	const layout = createMemo(() => buildLayout(props.blocks, props.team));
	let drag:
		| { pointerId: number; x: number; y: number; panX: number; panY: number }
		| undefined;

	const fit = (showAll = false) => {
		const element = board();
		if (!element) return;
		const rect = element.getBoundingClientRect();
		if (rect.width < 32 || rect.height < 32) return;
		const current = layout();
		const maxX = Math.max(
			...current.nodes.map(
				(node) => Math.abs(node.x - current.rootX) + current.cardWidth / 2,
			),
		);
		const maxY = Math.max(
			...current.nodes.map(
				(node) => Math.abs(node.y - current.rootY) + current.cardHeight / 2,
			),
		);
		const allZoom = Math.max(
			0.12,
			Math.min((rect.width - 96) / (maxX * 2), (rect.height - 96) / (maxY * 2), 1.4),
		);
		// Open at a readable scale around the centre. "Show all" zooms out far
		// enough to see every branch; the wheel and +/- go in both directions.
		const next = showAll ? allZoom : Math.max(allZoom, 0.8);
		setZoom(next);
		setPan({
			x: rect.width / 2 - current.rootX * next,
			y: rect.height / 2 - current.rootY * next,
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
		element.addEventListener("wheel", wheel, { passive: false });
		const observer = new ResizeObserver(() => fit());
		observer.observe(element);
		fit();
		onCleanup(() => {
			element.removeEventListener("wheel", wheel);
			observer.disconnect();
		});
	});

	const pointerDown = (event: PointerEvent) => {
		if ((event.target as HTMLElement).closest("a, button")) return;
		drag = {
			pointerId: event.pointerId,
			x: event.clientX,
			y: event.clientY,
			panX: pan().x,
			panY: pan().y,
		};
		setDragging(true);
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
	};
	const pointerMove = (event: PointerEvent) => {
		if (!drag || drag.pointerId !== event.pointerId) return;
		setPan({
			x: drag.panX + event.clientX - drag.x,
			y: drag.panY + event.clientY - drag.y,
		});
	};
	const pointerUp = () => {
		drag = undefined;
		setDragging(false);
	};

	return (
		<section class={s.boardSection} aria-label={m.plannerBoard()}>
			<div class={s.boardToolbar}>
				<Button
					aria-label={m.plannerZoomOut()}
					onClick={() => zoomBy(0.83)}
				>
					−
				</Button>
				<span class={s.zoomValue}>{Math.round(zoom() * 100)}%</span>
				<Button
					aria-label={m.plannerZoomIn()}
					onClick={() => zoomBy(1.2)}
				>
					+
				</Button>
				<Button onClick={() => fit(true)}>{m.plannerZoomFit()}</Button>
				<Button onClick={() => fit(false)}>{m.plannerZoomReset()}</Button>
			</div>
			<div
				class={s.board}
				ref={setBoard}
				data-dragging={dragging()}
				onPointerDown={pointerDown}
				onPointerMove={pointerMove}
				onPointerUp={pointerUp}
				onPointerCancel={pointerUp}
			>
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
						</defs>
						<For each={layout().edges}>
							{(edge) => (
								<path
									d={edge.path}
									class={s.edge}
									marker-end="url(#planner-arrow)"
								/>
							)}
						</For>
					</svg>
					<For each={layout().nodes}>
						{(node) => <PlannerNode node={node} team={props.team} />}
					</For>
				</div>
			</div>
			<p class={s.boardHint}>{m.plannerBoardHint()}</p>
		</section>
	);
}

export default function PlannerPage() {
	const { config } = useSiteConfig();
	const onPlannerHost = () => {
		const base = config()?.plannerBaseUrl;
		if (base) {
			try {
				return window.location.hostname === new URL(base).hostname;
			} catch {
				return false;
			}
		}
		return window.location.hostname.startsWith("planner.");
	};
	const [planner, { refetch }] = createResource(
		// Solid's createResource treats `false` as "no source" and skips the
		// fetcher, so the mode is a string rather than a boolean.
		() => (config.loading ? undefined : onPlannerHost() ? "public" : "team"),
		async (mode): Promise<PlannerView> => {
			if (mode === "team") {
				try {
					unwrap(await api.GET("/api/me"));
				} catch {
					return {
						team: false,
						blocks: unwrap(await api.GET("/api/public/planner")) ?? [],
					};
				}
				try {
					return {
						team: true,
						blocks: unwrap(await api.GET("/api/planner")) ?? [],
					};
				} catch {
					// Fall through to the public projection.
				}
			}
			return {
				team: false,
				blocks: unwrap(await api.GET("/api/public/planner")) ?? [],
			};
		},
	);
	const team = () => planner()?.team === true;
	return (
		<main class={s.page}>
			<header class={s.header}>
				<div>
					<h1>{m.plannerPageTitle()}</h1>
					<p>{team() ? m.plannerTeamHint() : m.plannerPublicHint()}</p>
				</div>
				<div class={s.headerActions}>
					<Show when={team()}>
						<A href="/coding">{m.codingPageTitle()}</A>
					</Show>
					<Button disabled={planner.loading} onClick={() => void refetch()}>
						{m.retry()}
					</Button>
				</div>
			</header>
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
						/>
					</Show>
				</Show>
			</Show>
		</main>
	);
}

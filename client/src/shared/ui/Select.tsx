import { createEffect, createSignal, For, onCleanup, onMount, Show } from "solid-js";
import { cx } from "@/shared/lib/cx";
import s from "./Select.module.css";

export type SelectOption = { value: string; label: string };

/**
 * Listbox-паттерн без нативного <select>: триггер-кнопка и всплывающий список.
 * Клавиатура: Enter/Space/ArrowDown — открыть, стрелки/Home/End — навигация,
 * Enter — выбор, Esc/Tab — закрыть. Клик вне — закрыть.
 */
export function Select(props: {
	value: string;
	options: SelectOption[];
	onChange: (value: string) => void;
	label: string;
	disabled?: boolean;
	class?: string;
}) {
	const [open, setOpen] = createSignal(false);
	const [active, setActive] = createSignal(0);
	let root!: HTMLDivElement;
	let listRef: HTMLDivElement | undefined;

	const selectedIndex = () =>
		Math.max(
			0,
			props.options.findIndex((option) => option.value === props.value),
		);
	const selected = () => props.options[selectedIndex()];

	createEffect(() => {
		if (open()) {
			setActive(selectedIndex());
			listRef?.focus();
		}
	});

	createEffect(() => {
		if (!open()) return;
		const node = listRef?.children[active()];
		if (node instanceof HTMLElement) node.scrollIntoView({ block: "nearest" });
	});

	function choose(index: number) {
		const option = props.options[index];
		if (!option) return;
		props.onChange(option.value);
		setOpen(false);
	}

	function onTriggerKey(event: KeyboardEvent) {
		if (props.disabled) return;
		if (event.key === "Enter" || event.key === " " || event.key === "ArrowDown") {
			event.preventDefault();
			setOpen(true);
		}
	}

	function onListKey(event: KeyboardEvent) {
		switch (event.key) {
			case "Escape":
				event.preventDefault();
				setOpen(false);
				break;
			case "ArrowDown":
				event.preventDefault();
				setActive(Math.min(props.options.length - 1, active() + 1));
				break;
			case "ArrowUp":
				event.preventDefault();
				setActive(Math.max(0, active() - 1));
				break;
			case "Home":
				event.preventDefault();
				setActive(0);
				break;
			case "End":
				event.preventDefault();
				setActive(props.options.length - 1);
				break;
			case "Enter":
			case " ":
				event.preventDefault();
				choose(active());
				break;
			case "Tab":
				setOpen(false);
				break;
		}
	}

	function onDocumentPointerDown(event: PointerEvent) {
		if (!open()) return;
		if (!root.contains(event.target as Node)) setOpen(false);
	}

	onMount(() => {
		document.addEventListener("pointerdown", onDocumentPointerDown);
		onCleanup(() => document.removeEventListener("pointerdown", onDocumentPointerDown));
	});

	return (
		<div ref={root} class={cx(s.root, props.class)}>
			<button
				type="button"
				class={s.trigger}
				role="combobox"
				aria-expanded={open()}
				aria-haspopup="listbox"
				aria-label={props.label}
				disabled={props.disabled}
				onClick={() => setOpen(!open())}
				onKeyDown={onTriggerKey}
			>
				<span class={s.value}>{selected()?.label ?? ""}</span>
				<svg
					class={s.chevron}
					classList={{ [s.chevronOpen]: open() }}
					aria-hidden="true"
					width="14"
					height="14"
					viewBox="0 0 16 16"
					fill="none"
				>
					<path
						d="M3.5 6l4.5 4.5L12.5 6"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"
					/>
				</svg>
			</button>
			<Show when={open()}>
				<div
					ref={listRef}
					role="listbox"
					aria-label={props.label}
					class={s.list}
					data-elevated
					tabIndex={-1}
					onKeyDown={onListKey}
				>
					<For each={props.options}>
						{(option, index) => (
							// biome-ignore lint/a11y/useFocusableInteractive: фокус управляется на уровне списка
							// biome-ignore lint/a11y/useKeyWithClickEvents: клавиатура обработана на listbox
							<div
								role="option"
								aria-selected={option.value === props.value}
								class={s.option}
								classList={{
									[s.optionSelected]: option.value === props.value,
									[s.optionActive]: index() === active(),
								}}
								onMouseMove={() => setActive(index())}
								onClick={() => choose(index())}
							>
								<span class={s.optionLabel}>{option.label}</span>
								<Show when={option.value === props.value}>
									<svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none">
										<path
											d="M3 8.5l3.5 3.5L13 4.5"
											stroke="currentColor"
											stroke-width="1.8"
											stroke-linecap="round"
											stroke-linejoin="round"
										/>
									</svg>
								</Show>
							</div>
						)}
					</For>
				</div>
			</Show>
		</div>
	);
}

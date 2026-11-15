import { createUniqueId, type JSX, Show, splitProps } from "solid-js";
import { cx } from "@/shared/lib/cx";
import s from "./Input.module.css";

export type InputProps = JSX.InputHTMLAttributes<HTMLInputElement> & {
	label: string;
	hint?: string;
	error?: string;
};

export function Input(props: InputProps) {
	const [local, rest] = splitProps(props, [
		"class",
		"label",
		"hint",
		"error",
		"id",
	]);
	const generatedId = createUniqueId();
	const id = () => local.id ?? generatedId;
	return (
		<div class={cx(s.root, local.class)}>
			<label class={s.label} for={id()}>
				{local.label}
			</label>
			<input
				{...rest}
				id={id()}
				class={s.control}
				aria-invalid={local.error ? true : undefined}
				aria-describedby={local.hint ? `${id()}-hint` : undefined}
			/>
			<Show when={local.hint}>
				<span id={`${id()}-hint`} class={s.hint}>
					{local.hint}
				</span>
			</Show>
			<Show when={local.error}>
				<span role="alert" class={s.error}>
					{local.error}
				</span>
			</Show>
		</div>
	);
}

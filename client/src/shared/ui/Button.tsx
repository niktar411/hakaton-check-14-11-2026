import { type JSX, splitProps } from "solid-js";
import { cx } from "@/shared/lib/cx";
import s from "./Button.module.css";

export type ButtonProps = JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
	variant?: "primary" | "secondary" | "danger";
};

export function Button(props: ButtonProps) {
	const [local, rest] = splitProps(props, ["children", "class", "variant"]);
	return (
		<button
			type="button"
			class={cx(
				s.root,
				local.variant === "primary" && s.primary,
				local.variant === "danger" && s.danger,
				local.class,
			)}
			{...rest}
		>
			{local.children}
		</button>
	);
}

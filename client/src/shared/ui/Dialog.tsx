import { createEffect, onCleanup, type JSX } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

export function Dialog(props: {
	open: boolean;
	title: string;
	onClose: () => void;
	children: JSX.Element;
}) {
	let dialog: HTMLDialogElement | undefined;
	createEffect(() => {
		if (props.open && !dialog?.open) dialog?.showModal();
		else if (!props.open && dialog?.open) dialog.close();
	});
	onCleanup(() => dialog?.close());
	return (
		<dialog
			ref={dialog}
			aria-label={props.title}
			onCancel={(event) => {
				event.preventDefault();
				props.onClose();
			}}
			class="border-border bg-surface text-text m-auto box-border max-h-dvh w-full max-w-2xl overflow-y-auto rounded-md border p-5"
			data-elevated
		>
			<div class="mb-4 flex items-start justify-between gap-4">
				<h2 class="text-xl font-bold">{props.title}</h2>
				<Button aria-label={m.close()} onClick={props.onClose}>
					{m.close()}
				</Button>
			</div>
			{props.children}
		</dialog>
	);
}

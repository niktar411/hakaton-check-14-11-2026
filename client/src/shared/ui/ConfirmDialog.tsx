import type { JSX } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";
import { Dialog } from "@/shared/ui/Dialog";

export function ConfirmDialog(props: {
	open: boolean;
	title: string;
	description: string;
	confirmLabel?: string;
	pending?: boolean;
	confirmDisabled?: boolean;
	danger?: boolean;
	onConfirm: () => void;
	onClose: () => void;
	children?: JSX.Element;
}) {
	return (
		<Dialog open={props.open} title={props.title} onClose={props.onClose}>
			<div class="grid min-w-0 gap-4">
				<p class="whitespace-pre-wrap">{props.description}</p>
				{props.children}
				<div class="flex flex-wrap justify-end gap-3">
					<Button disabled={props.pending} onClick={props.onClose}>
						{m.cancel()}
					</Button>
					<Button
						variant={props.danger ? "danger" : "primary"}
						disabled={props.pending || props.confirmDisabled}
						onClick={props.onConfirm}
					>
						{props.pending ? m.loading() : (props.confirmLabel ?? m.apply())}
					</Button>
				</div>
			</div>
		</Dialog>
	);
}

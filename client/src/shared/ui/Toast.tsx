import { createEffect, createSignal, onCleanup, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

type ToastState = {
	text: string;
	duration: number;
};

const [toast, setToast] = createSignal<ToastState | null>(null);

export function useToast() {
	return {
		show: (text: string, duration = 3000) => setToast({ text, duration }),
		clear: () => setToast(null),
	};
}

export function Toast() {
	createEffect(() => {
		const value = toast();
		if (!value) return;
		const timer = window.setTimeout(() => setToast(null), value.duration);
		onCleanup(() => window.clearTimeout(timer));
	});
	return (
		<Show when={toast()}>
			{(value) => (
				<div
					role="status"
					class="border-border bg-surface-2 fixed inset-x-4 bottom-4 z-50 flex items-center justify-between gap-4 rounded-sm border p-4 md:right-6 md:bottom-6 md:left-auto md:max-w-lg"
					data-elevated
				>
					<span>{value().text}</span>
					<Button onClick={() => setToast(null)}>{m.close()}</Button>
				</div>
			)}
		</Show>
	);
}

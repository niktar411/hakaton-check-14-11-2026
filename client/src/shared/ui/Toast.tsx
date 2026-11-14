import { createSignal, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

const [message, setMessage] = createSignal("");

export function useToast() {
	return { show: setMessage, clear: () => setMessage("") };
}

export function Toast() {
	return (
		<Show when={message()}>
			<div
				role="status"
				class="border-border bg-surface-2 fixed inset-x-4 bottom-4 z-50 flex items-center justify-between gap-4 rounded-sm border p-4 md:right-6 md:bottom-6 md:left-auto md:max-w-lg"
				data-elevated
			>
				<span>{message()}</span>
				<Button onClick={() => setMessage("")}>{m.close()}</Button>
			</div>
		</Show>
	);
}

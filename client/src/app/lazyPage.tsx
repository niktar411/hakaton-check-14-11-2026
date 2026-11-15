import type { Component } from "solid-js";
import { Dynamic } from "solid-js/web";
import {
	createResource,
	ErrorBoundary,
	Show,
	Suspense,
} from "solid-js";
import { LoadingState } from "@/shared/ui/State";

/**
 * Code-split route loader. The chunk is imported on the first render of its
 * route — not on app startup — so each screen ships in its own file.
 * Replaces @solidjs/router's `lazy`, which is no longer exported in v1.
 */
export function lazyPage(
	load: () => Promise<{ default: Component<Record<string, unknown>> }>,
): Component<Record<string, unknown>> {
	let promise: Promise<Component<Record<string, unknown>>> | undefined;
	return function LazyPage(props: Record<string, unknown>) {
		const [component] = createResource(async () => {
			if (!promise) promise = load().then((m) => m.default);
			return promise;
		});
		return (
			<Suspense fallback={<LoadingState />}>
				<ErrorBoundary fallback={(err) => <div class="text-text p-8">{String(err)}</div>}>
					<Show when={component()}>
						{(loaded) => <Dynamic component={loaded()} {...props} />}
					</Show>
				</ErrorBoundary>
			</Suspense>
		);
	};
}

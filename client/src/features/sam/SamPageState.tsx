import {
	createSignal,
	type JSX,
	Match,
	onMount,
	Switch,
} from "solid-js";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";

/**
 * Minimal resource shape used by SAM screens. It is deliberately smaller than
 * Solid's Resource: only the four data states and refetch are needed here.
 */
export type SamResource<T> = {
	(): T | undefined;
	readonly loading: boolean;
	readonly error: unknown;
};

/**
 * Loads data into explicit signals instead of a rejected Resource promise.
 * A failed request must always reach the error branch, including on the first
 * render, so the screen never stays in the loading skeleton.
 */
export function createSamResource<T>(loader: () => Promise<T>) {
	const [data, setData] = createSignal<T>();
	const [error, setError] = createSignal<unknown>();
	const [loading, setLoading] = createSignal(true);
	let request = 0;

	const refetch = async () => {
		const current = ++request;
		setLoading(true);
		setError(undefined);
		try {
			const value = await loader();
			if (current === request) setData(() => value);
		} catch (reason) {
			if (current === request) setError(reason);
		} finally {
			if (current === request) setLoading(false);
		}
	};

	onMount(() => {
		void refetch();
	});

	const resource = (() => data()) as SamResource<T>;
	Object.defineProperties(resource, {
		loading: { get: loading },
		error: { get: error },
	});
	return { resource, refetch };
}

/**
 * Renders the four data states shared by every SAM screen. The success branch
 * only runs after data arrives, so pages never render partial results.
 */
export function SamPageState<T>(props: {
	data: SamResource<T>;
	empty?: (value: T) => boolean;
	onRetry: () => void;
	emptyMessage?: string;
	children: (value: T) => JSX.Element;
}) {
	const isEmpty = () => {
		const value = props.data();
		return value !== undefined && props.empty !== undefined
			? props.empty(value)
			: false;
	};
	return (
		<Switch>
			<Match when={props.data.loading}>
				<LoadingState />
			</Match>
			<Match when={props.data.error}>
				<ErrorState onRetry={props.onRetry} />
			</Match>
			<Match when={isEmpty()}>
				<EmptyState message={props.emptyMessage} />
			</Match>
			<Match when={props.data()}>
				{(value) => props.children(value())}
			</Match>
		</Switch>
	);
}

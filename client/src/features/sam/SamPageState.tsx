import {
	createResource,
	type JSX,
	type Resource,
	Match,
	Switch,
} from "solid-js";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";

/**
 * Loads data into Solid's native resource so every SAM screen shares the same
 * loading, error, empty and success behaviour.
 */
export function createSamResource<T>(loader: () => Promise<T>) {
	const [resource, { refetch }] = createResource(loader);
	return { resource, refetch };
}

/**
 * Renders the four data states shared by every SAM screen. The success branch
 * only runs after data arrives, so pages never render partial results.
 */
export function SamPageState<T>(props: {
	data: Resource<T>;
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


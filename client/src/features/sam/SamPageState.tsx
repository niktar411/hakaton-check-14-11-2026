import { type JSX, Match, type Resource, Switch } from "solid-js";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";

/**
 * Renders the four data states shared by every SAM screen. The success branch
 * only runs after the resource resolved, so pages never render partial data.
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

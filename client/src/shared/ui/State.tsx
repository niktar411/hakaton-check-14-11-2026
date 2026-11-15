import * as m from "@/paraglide/messages";
import { Button } from "@/shared/ui/Button";

export function LoadingState(props: { label?: string }) {
	return (
		<div class="bg-surface-2 animate-pulse rounded-md p-6" aria-busy="true">
			{props.label ?? m.loading()}
		</div>
	);
}

export function ErrorState(props: { onRetry?: () => void }) {
	return (
		<div class="border-danger rounded-sm border p-4" role="alert">
			<p>{m.loadError()}</p>
			{props.onRetry && (
				<Button class="mt-3" onClick={props.onRetry}>
					{m.retry()}
				</Button>
			)}
		</div>
	);
}

export function EmptyState(props: { message?: string }) {
	return <p class="text-muted py-4">{props.message ?? m.empty()}</p>;
}

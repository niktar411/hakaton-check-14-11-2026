import { createResource, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, downloadText, unwrap } from "@/shared/api";
import { Button } from "@/shared/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "@/shared/ui/State";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

export function MihomoPanel() {
	const toast = useToast();
	const [config, { refetch }] = createResource(async () =>
		unwrap(await api.GET("/api/admin/mihomo")),
	);

	async function copy() {
		try {
			await navigator.clipboard.writeText(config()?.configuration ?? "");
			toast.show(m.copied());
		} catch {
			toast.show(m.copyFailed());
		}
	}

	return (
		<section class={s.panel}>
			<div><h2>{m.adminMihomo()}</h2><p class={s.muted}>{m.mihomoHint()}</p></div>
			<Show when={!config.loading} fallback={<LoadingState />}>
				<Show when={!config.error} fallback={<ErrorState onRetry={() => void refetch()} />}>
					<Show when={config()?.configuration} fallback={<EmptyState message={m.mihomoEmpty()} />}>
						{(configuration) => (
							<>
								<pre class={s.yaml} data-testid="mihomo-configuration">{configuration()}</pre>
								<div class={s.actions}>
									<Button onClick={() => void copy()}>{m.copyConfiguration()}</Button>
									<Button onClick={() => downloadText("mihomo.yaml", configuration(), "application/yaml")}>{m.downloadConfiguration()}</Button>
									<Button onClick={() => void refetch()}>{m.retry()}</Button>
								</div>
							</>
						)}
					</Show>
				</Show>
			</Show>
		</section>
	);
}

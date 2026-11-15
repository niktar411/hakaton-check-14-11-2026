import { createEffect, createSignal, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, csrfHeaders, unwrap } from "@/shared/api";
import { Button } from "@/shared/ui/Button";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";
import s from "./AdministrationPage.module.css";

export function ModelKeyPanel(props: { csrf?: string; configured?: boolean }) {
	const toast = useToast();
	const [key, setKey] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [configured, setConfigured] = createSignal<boolean | undefined>(
		props.configured,
	);
	createEffect(() => setConfigured(props.configured));

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!key().trim() || pending()) return;
		setPending(true);
		try {
			const result = unwrap(
				await api.PUT("/api/admin/model-key", {
					headers: csrfHeaders(props.csrf),
					body: { key: key().trim() },
				}),
			);
			setConfigured(result.configured);
			setKey("");
			toast.show(m.success());
		} catch {
			toast.show(m.loadError());
		} finally {
			setPending(false);
		}
	}

	return (
		<section class={s.panel}>
			<div>
				<h2>{m.adminModelKey()}</h2>
				<p class={s.muted}>{m.modelKeyHint()}</p>
			</div>
			<Show when={configured() !== undefined}>
				<p role="status">
					{configured() ? m.modelKeyConfigured() : m.modelUnavailable()}
				</p>
			</Show>
			<form class={s.form} onSubmit={save}>
				<Input
					label={m.newModelKey()}
					type="password"
					autocomplete="off"
					required
					value={key()}
					onInput={(event) => setKey(event.currentTarget.value)}
					hint={m.modelKeyNeverDisplay()}
				/>
				<Button
					type="submit"
					variant="primary"
					disabled={pending() || !key().trim()}
				>
					{pending() ? m.loading() : m.save()}
				</Button>
			</form>
		</section>
	);
}

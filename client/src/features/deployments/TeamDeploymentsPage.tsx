import { createResource, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";
import { DeploymentsPanel } from "@/features/administration/DeploymentsPanel";
import { LocalLoginForm } from "@/features/auth/LocalLoginForm";
import { LoadingState } from "@/shared/ui/State";

type Identity = components["schemas"]["Identity"];

export default function TeamDeploymentsPage() {
	const [session, { refetch }] = createResource(async () => {
		try {
			return unwrap(await api.GET("/api/me")) as Identity;
		} catch {
			return null;
		}
	});
	return (
		<main class="mx-auto flex w-full max-w-7xl flex-col gap-6 p-4 md:p-8">
			<header>
				<h1>{m.teamDeployments()}</h1>
				<p class="text-text-muted">{m.teamDeploymentsHint()}</p>
			</header>
			<Show when={!session.loading} fallback={<LoadingState />}>
				<Show
					when={session()}
					fallback={
						<section class="border-border mx-auto w-full max-w-md rounded-md border p-5">
							<h2>{m.teamLoginTitle()}</h2>
							<p>{m.codingLoginHint()}</p>
							<LocalLoginForm
								audience="coding"
								onSuccess={() => void refetch()}
							/>
						</section>
					}
				>
					<DeploymentsPanel member csrf={session()?.csrf} />
				</Show>
			</Show>
		</main>
	);
}

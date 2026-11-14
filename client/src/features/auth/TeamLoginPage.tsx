import * as m from "@/paraglide/messages";
import { LocalLoginForm } from "@/features/auth/LocalLoginForm";

export function TeamLoginPage(props: { onSuccess?: () => void }) {
	return (
		<main class="flex min-h-dvh items-center justify-center p-4">
			<section class="bg-surface border-border w-full max-w-md rounded-md border p-6">
				<h1 class="m-0">{m.teamLoginTitle()}</h1>
				<p class="text-muted">{m.teamLoginHint()}</p>
				<LocalLoginForm audience="coding" onSuccess={() => props.onSuccess?.()} />
			</section>
		</main>
	);
}

export default TeamLoginPage;

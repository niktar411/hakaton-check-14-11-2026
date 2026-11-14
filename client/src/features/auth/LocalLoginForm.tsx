import { createSignal, Show } from "solid-js";
import * as m from "@/paraglide/messages";
import { api, ApiError, unwrap } from "@/shared/api";
import type { components } from "@/shared/api/schema";

type Audience = components["schemas"]["LoginRequest"]["audience"];
type LoginResult = components["schemas"]["LoginResult"];
import { Button } from "@/shared/ui/Button";
import { Input } from "@/shared/ui/Input";
import { useToast } from "@/shared/ui/Toast";

export function LocalLoginForm(props: {
	audience: Audience;
	onSuccess: (session: LoginResult) => void;
}) {
	const toast = useToast();
	const [username, setUsername] = createSignal("");
	const [password, setPassword] = createSignal("");
	const [pending, setPending] = createSignal(false);
	const [failed, setFailed] = createSignal(false);
	const [problem, setProblem] = createSignal("");

	async function login(event: SubmitEvent) {
		event.preventDefault();
		if (pending() || !username().trim() || !password()) return;
		setPending(true);
		setFailed(false);
		setProblem("");
		try {
			const session = unwrap(
				await api.POST("/api/auth/local-login", {
					body: {
						username: username().trim(),
						password: password(),
						audience: props.audience,
					},
				}),
			);
			setPassword("");
			props.onSuccess(session);
			toast.show(m.loginSuccess());
		} catch (error) {
			const message =
				error instanceof ApiError && error.code === "auth.admin_vpn_required"
					? m.adminVpnRequired()
					: error instanceof ApiError && error.status === 429
						? m.loginTooManyAttempts()
						: m.loginFailed();
			setFailed(true);
			setProblem(message);
			toast.show(message);
		} finally {
			setPending(false);
		}
	}

	return (
		<form class="grid w-full max-w-md gap-4" onSubmit={login}>
			<Input
				label={m.username()}
				name="username"
				autocomplete="username"
				required
				value={username()}
				onInput={(event) => setUsername(event.currentTarget.value)}
			/>
			<Input
				label={m.password()}
				name="password"
				type="password"
				autocomplete="current-password"
				required
				value={password()}
				onInput={(event) => setPassword(event.currentTarget.value)}
			/>
			<Show when={failed()}>
				<p class="text-danger" role="alert">
					{problem() || m.loginFailed()}
				</p>
			</Show>
			<Button
				type="submit"
				variant="primary"
				disabled={pending() || !username().trim() || !password()}
			>
				{pending() ? m.loading() : m.localLogin()}
			</Button>
		</form>
	);
}

import { useNavigate } from "@solidjs/router";
import * as m from "@/paraglide/messages";
import { LocalLoginForm } from "@/features/auth/LocalLoginForm";
import s from "./AdminLoginPage.module.css";

export default function AdminLoginPage() {
	const navigate = useNavigate();
	return (
		<main class={s.page}>
			<section class={s.card}>
				<h1>{m.adminLoginPageTitle()}</h1>
				<p>{m.adminLoginHint()}</p>
				<LocalLoginForm
					audience="admin"
					onSuccess={() => navigate("/admin", { replace: true })}
				/>
			</section>
		</main>
	);
}
